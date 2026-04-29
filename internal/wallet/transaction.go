package wallet

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"eth-scan/pkg/ethclient"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// NonceManager 本地 Nonce 计数器，解决高并发下 PendingNonceAt 延迟问题
// 生产环境建议迁移到 Redis 以支持多实例部署
type NonceManager struct {
	mu     sync.Mutex
	nonces map[string]uint64 // address (lowercase hex) → next nonce
}

var globalNonceManager = &NonceManager{
	nonces: make(map[string]uint64),
}

// GetNonce 获取地址下一个可用 Nonce，优先从本地缓存取，首次从链上同步
func (m *NonceManager) GetNonce(ctx context.Context, address common.Address) (uint64, error) {
	key := address.Hex()
	m.mu.Lock()
	defer m.mu.Unlock()

	if nonce, ok := m.nonces[key]; ok {
		m.nonces[key]++
		return nonce, nil
	}
	// 首次：从链上同步
	nonce, err := ethclient.GetInstance().GetTransactionCount(ctx, address)
	if err != nil {
		return 0, fmt.Errorf("获取链上 Nonce 失败: %w", err)
	}
	m.nonces[key] = nonce + 1
	return nonce, nil
}

// RollbackNonce 交易失败时回滚 Nonce，使下次重试可以复用该 Nonce
func (m *NonceManager) RollbackNonce(address common.Address) {
	key := address.Hex()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nonces[key] > 0 {
		m.nonces[key]--
	}
}

// SyncNonce 强制从链上重新同步（节点重启或 Nonce 不一致时使用）
func (m *NonceManager) SyncNonce(ctx context.Context, address common.Address) error {
	nonce, err := ethclient.GetInstance().GetTransactionCount(ctx, address)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.nonces[address.Hex()] = nonce
	m.mu.Unlock()
	return nil
}

// SendETH 从 fromPrivKeyHex 地址向 to 发送 ETH
// amount 单位 Wei，chainID 用于 EIP-155 签名
func SendETH(fromPrivKeyHex string, to common.Address, amount *big.Int, chainID *big.Int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	privateKey, err := crypto.HexToECDSA(fromPrivKeyHex)
	if err != nil {
		return "", fmt.Errorf("私钥解析失败: %w", err)
	}
	from := crypto.PubkeyToAddress(privateKey.PublicKey)

	nonce, err := globalNonceManager.GetNonce(ctx, from)
	if err != nil {
		return "", err
	}

	gasPrice, err := ethclient.GetInstance().GetSuggestGasPrice(ctx)
	if err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("获取 GasPrice 失败: %w", err)
	}

	gasLimit, err := ethclient.GetInstance().EstimateGas(ctx, ethereum.CallMsg{
		From:  from,
		To:    &to,
		Value: amount,
	})
	if err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("预估 Gas 失败: %w", err)
	}

	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)
	signer := types.NewEIP155Signer(chainID)
	signedTx, err := types.SignTx(tx, signer, privateKey)
	if err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("交易签名失败: %w", err)
	}

	if err = ethclient.GetInstance().SendRawTransaction(ctx, signedTx); err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("广播失败: %w", err)
	}
	return signedTx.Hash().Hex(), nil
}

// SendERC20 从 fromPrivKeyHex 地址向 to 发送 ERC20 代币
// contractAddr: 合约地址；amount: 代币最小单位数量
func SendERC20(fromPrivKeyHex string, contractAddr, to common.Address, amount *big.Int, callData []byte, chainID *big.Int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	privateKey, err := crypto.HexToECDSA(fromPrivKeyHex)
	if err != nil {
		return "", fmt.Errorf("私钥解析失败: %w", err)
	}
	from := crypto.PubkeyToAddress(privateKey.PublicKey)

	nonce, err := globalNonceManager.GetNonce(ctx, from)
	if err != nil {
		return "", err
	}

	gasPrice, err := ethclient.GetInstance().GetSuggestGasPrice(ctx)
	if err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", err
	}

	gasLimit, err := ethclient.GetInstance().EstimateGas(ctx, ethereum.CallMsg{
		From: from,
		To:   &contractAddr,
		Data: callData,
	})
	if err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("预估 Gas 失败: %w", err)
	}

	// ERC20 转账 tx.Value = 0，金额在 callData 中
	tx := types.NewTransaction(nonce, contractAddr, big.NewInt(0), gasLimit, gasPrice, callData)
	signer := types.NewEIP155Signer(chainID)
	signedTx, err := types.SignTx(tx, signer, privateKey)
	if err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("交易签名失败: %w", err)
	}

	if err = ethclient.GetInstance().SendRawTransaction(ctx, signedTx); err != nil {
		globalNonceManager.RollbackNonce(from)
		return "", fmt.Errorf("广播失败: %w", err)
	}
	return signedTx.Hash().Hex(), nil
}
