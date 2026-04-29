package wallet

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"eth-scan/internal/contract"
	"eth-scan/pkg/ethclient"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

// GetEthBalance 查询地址的 ETH 余额（单位 Wei）
func GetEthBalance(address string) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ethclient.GetInstance().GetEthBalance(ctx, address)
}

// GetERC20Balance 查询地址在指定 ERC20 合约中的代币余额（单位：代币最小单位）
// contractAddr: 合约地址（如 USDT 合约）
// walletAddr: 查询余额的钱包地址
func GetERC20Balance(contractAddr, walletAddr string) (*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	to := common.HexToAddress(contractAddr)
	owner := common.HexToAddress(walletAddr)

	callData := contract.EncodeBalanceOf(owner)
	result, err := ethclient.GetInstance().CallContract(ctx, ethereum.CallMsg{
		To:   &to,
		Data: callData,
	})
	if err != nil {
		return nil, fmt.Errorf("查询 ERC20 余额失败: %w", err)
	}
	return contract.DecodeUint256(result), nil
}
