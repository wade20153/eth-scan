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
	return e.GetClient().BlockNumber(ctx)
}

// GetBlockByNumber 根据高度获取完整区块数据（含所有交易）
func (e *Eth) GetBlockByNumber(ctx context.Context, number uint64) (*types.Block, error) {
	return e.GetClient().BlockByNumber(ctx, big.NewInt(int64(number)))
}

// GetTransactionReceipt 获取交易回执，用于确认状态（Status==1）和解析 Event Logs
func (e *Eth) GetTransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return e.GetClient().TransactionReceipt(ctx, txHash)
}

// GetEthBalance 查询地址的 ETH 原生余额（单位 Wei）
func (e *Eth) GetEthBalance(ctx context.Context, address string) (*big.Int, error) {
	return e.GetClient().BalanceAt(ctx, common.HexToAddress(address), nil)
}

// CallContract 执行只读合约调用（如 ERC20 balanceOf）
func (e *Eth) CallContract(ctx context.Context, msg ethereum.CallMsg) ([]byte, error) {
	return e.GetClient().CallContract(ctx, msg, nil)
}

// GetSuggestGasPrice 获取当前网络建议的 GasPrice（单位 Wei）
func (e *Eth) GetSuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return e.GetClient().SuggestGasPrice(ctx)
}

// GetTransactionCount 获取地址的 Nonce（pending 状态，包含未打包交易）
func (e *Eth) GetTransactionCount(ctx context.Context, address common.Address) (uint64, error) {
	return e.GetClient().PendingNonceAt(ctx, address)
}

// EstimateGas 预估交易所需 Gas Limit
func (e *Eth) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error) {
	return e.GetClient().EstimateGas(ctx, msg)
}

// SendRawTransaction 广播已签名的原始交易
func (e *Eth) SendRawTransaction(ctx context.Context, tx *types.Transaction) error {
	return e.GetClient().SendTransaction(ctx, tx)
}

// GetLogs 按过滤条件获取事件日志（用于 ERC20 Transfer 事件扫描）
func (e *Eth) GetLogs(ctx context.Context, query ethereum.FilterQuery) ([]types.Log, error) {
	return e.GetClient().FilterLogs(ctx, query)
}

// GetCode 获取地址的合约字节码，返回 0x 表示普通钱包地址
func (e *Eth) GetCode(ctx context.Context, address common.Address) ([]byte, error) {
	return e.GetClient().CodeAt(ctx, address, nil)
}

// GetChainID 获取当前网络 Chain ID（用于 EIP-155 重放保护签名）
func (e *Eth) GetChainID(ctx context.Context) (*big.Int, error) {
	return e.GetClient().ChainID(ctx)
}
