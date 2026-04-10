package ethclient

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// GetLatestBlockNumber 获取当前以太坊网络的最新区块高度
func (e *Eth) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	client := e.GetClient()
	return client.BlockNumber(ctx)
}

// GetBlockByNumber 根据高度获取完整区块数据 (包含该块下的所有 Transactions)
func (e *Eth) GetBlockByNumber(ctx context.Context, number uint64) (*types.Block, error) {
	client := e.GetClient()
	newInt := big.NewInt(int64(number))
	return client.BlockByNumber(ctx, newInt)
}

// GetTransactionReceipt 获取交易回执
// 扫块必查：用来判断交易是否成功（Status == 1）以及获取 Event Logs（解析充值金额）
func (e *Eth) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	client := e.GetClient()
	return client.TransactionReceipt(ctx, txHash)
}

// GetEthBalance 查询指定地址的 ETH 原生余额
func (e *Eth) GetEthBalance(ctx context.Context, address string) (*big.Int, error) {
	toAddress := common.HexToAddress(address)
	return e.GetClient().BalanceAt(ctx, toAddress, nil) // nil 表示查询最新确认的余额
}

// CallContract 执行一个只读的合约调用 (如查询 ERC20 的 balanceOf)
// 这是 internal/wallet/balance.go 中查询代币余额的基础
func (e *Eth) CallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	return e.GetClient().CallContract(ctx, msg, nil)
}

// GetSuggestGasPrice 获取当前网络建议的 Gas 价格
func (e *Eth) GetSuggestGasPrice(ctx context.Context) (*big.Int, error) {
	client := e.GetClient()
	return client.SuggestGasPrice(ctx)
}
