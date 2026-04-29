package sweep

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"eth-scan/config"
	"eth-scan/internal/contract"
	"eth-scan/internal/wallet"
	"eth-scan/pkg/ethclient"
	"eth-scan/repository"
)

const gasReserveWei = int64(500_000_000_000_000)     // ETH 归集预留 Gas：0.0005 ETH
const erc20GasSupplyWei = int64(300_000_000_000_000) // 补给子地址的 Gas：0.0003 ETH
const confirmPollInterval = 3 * time.Second
const confirmTimeout = 60 * time.Second

// Sweeper 资金归集器
type Sweeper struct {
	cfg        *config.Config
	hotWallet  common.Address
	hotPrivKey string
}

func NewSweeper(cfg *config.Config) (*Sweeper, error) {
	if cfg.HotWallet.Address == "" || cfg.HotWallet.PrivateKey == "" {
		return nil, fmt.Errorf("hot_wallet 配置缺失，请在 config.yaml 填写 address 和 private_key")
	}
	return &Sweeper{
		cfg:        cfg,
		hotWallet:  common.HexToAddress(cfg.HotWallet.Address),
		hotPrivKey: cfg.HotWallet.PrivateKey,
	}, nil
}

// Sweep 归集入口，根据代币类型分发，归集成功后更新用户余额和交易状态
func (s *Sweeper) Sweep(ctx context.Context, record *repository.Transaction, ethAddr *repository.EthAddress, mnemonic string) {
	privKeyHex, _, err := wallet.DerivePrivateKeyHex(mnemonic, ethAddr.DeriveIndex)
	if err != nil {
		log.Printf("[归集] 派生私钥失败 address=%s: %v", record.ToAddress, err)
		return
	}

	chainID, err := ethclient.GetInstance().GetChainID(ctx)
	if err != nil {
		log.Printf("[归集] 获取 ChainID 失败: %v", err)
		return
	}

	var sweepOK bool
	switch record.TokenType {
	case "ETH":
		sweepOK = s.sweepETH(ctx, record, privKeyHex, chainID)
	default:
		contractAddr := s.getContractAddress(record.TokenType)
		if contractAddr == "" {
			log.Printf("[归集] 未找到 %s 合约地址配置，跳过", record.TokenType)
			return
		}
		sweepOK = s.sweepERC20(ctx, record, privKeyHex, contractAddr, chainID)
	}

	if !sweepOK {
		return
	}

	// ★ 归集成功：给用户余额加上本次充值金额（原始充值额，Gas 损耗由平台承担）
	if err := repository.AddUserBalance(record.UserID, record.TokenType, record.Value); err != nil {
		log.Printf("[归集] 更新用户余额失败 userID=%d token=%s: %v", record.UserID, record.TokenType, err)
		return
	}

	// ★ 交易状态改为 2=已到账
	if err := repository.UpdateTransactionStatus(record.TxHash, 2); err != nil {
		log.Printf("[归集] 更新交易状态失败 txHash=%s: %v", record.TxHash, err)
		return
	}

	log.Printf("[到账] userID=%d token=%s amount=%s", record.UserID, record.TokenType, record.Value)
}

// sweepETH 归集 ETH，返回是否成功
func (s *Sweeper) sweepETH(ctx context.Context, record *repository.Transaction, childPrivKey string, chainID *big.Int) bool {
	balance, err := wallet.GetEthBalance(record.ToAddress)
	if err != nil {
		log.Printf("[归集-ETH] 查询余额失败 addr=%s: %v", record.ToAddress, err)
		return false
	}

	reserve := big.NewInt(gasReserveWei)
	if balance.Cmp(reserve) <= 0 {
		log.Printf("[归集-ETH] 余额不足以归集 addr=%s balance=%s", record.ToAddress, balance.String())
		return false
	}

	// 可归集金额 = 当前余额 - Gas 预留
	sweepAmount := new(big.Int).Sub(balance, reserve)
	txHash, err := wallet.SendETH(childPrivKey, s.hotWallet, sweepAmount, chainID)
	if err != nil {
		log.Printf("[归集-ETH] 转账失败 addr=%s: %v", record.ToAddress, err)
		return false
	}
	log.Printf("[归集-ETH] 成功 addr=%s sweepAmount=%s txHash=%s", record.ToAddress, sweepAmount.String(), txHash)
	return true
}

// sweepERC20 归集 USDT/USDC，返回是否成功
// 第一步：热钱包 → 子地址 补 Gas（ETH）
// 第二步：子地址 → 热钱包 转出全部 ERC20
func (s *Sweeper) sweepERC20(ctx context.Context, record *repository.Transaction, childPrivKey, contractAddr string, chainID *big.Int) bool {
	erc20Balance, err := wallet.GetERC20Balance(contractAddr, record.ToAddress)
	if err != nil || erc20Balance.Sign() == 0 {
		log.Printf("[归集-ERC20] %s 余额为 0 或查询失败 addr=%s", record.TokenType, record.ToAddress)
		return false
	}

	// 第一步：子地址 ETH 不足时，由热钱包补 Gas
	childETHBalance, _ := wallet.GetEthBalance(record.ToAddress)
	gasSupply := big.NewInt(erc20GasSupplyWei)
	if childETHBalance.Cmp(gasSupply) < 0 {
		toAddr := common.HexToAddress(record.ToAddress)
		gasTxHash, err := wallet.SendETH(s.hotPrivKey, toAddr, gasSupply, chainID)
		if err != nil {
			log.Printf("[归集-ERC20] 补 Gas 失败 addr=%s: %v", record.ToAddress, err)
			return false
		}
		log.Printf("[归集-ERC20] 补 Gas txHash=%s，等待确认...", gasTxHash)

		if err := s.waitForETHBalance(ctx, record.ToAddress, gasSupply); err != nil {
			log.Printf("[归集-ERC20] 等待 Gas 确认超时 addr=%s: %v", record.ToAddress, err)
			return false
		}
	}

	// 第二步：子地址将全部 ERC20 转到热钱包
	callData := contract.EncodeTransfer(s.hotWallet, erc20Balance)
	txHash, err := wallet.SendERC20(
		childPrivKey,
		common.HexToAddress(contractAddr),
		s.hotWallet,
		erc20Balance,
		callData,
		chainID,
	)
	if err != nil {
		log.Printf("[归集-ERC20] %s 转账失败 addr=%s: %v", record.TokenType, record.ToAddress, err)
		return false
	}
	log.Printf("[归集-ERC20] %s 归集成功 addr=%s amount=%s txHash=%s",
		record.TokenType, record.ToAddress, erc20Balance.String(), txHash)
	return true
}

// waitForETHBalance 轮询等待子地址 ETH 余额到达目标值
func (s *Sweeper) waitForETHBalance(ctx context.Context, addr string, target *big.Int) error {
	deadline := time.Now().Add(confirmTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		balance, err := wallet.GetEthBalance(addr)
		if err == nil && balance.Cmp(target) >= 0 {
			return nil
		}
		time.Sleep(confirmPollInterval)
	}
	return fmt.Errorf("等待超时（%v）", confirmTimeout)
}

// getContractAddress 根据代币符号从配置查合约地址
func (s *Sweeper) getContractAddress(symbol string) string {
	for _, c := range s.cfg.BlockChain.ERC20Contracts {
		if strings.EqualFold(c.Symbol, symbol) {
			return c.Address
		}
	}
	return ""
}
