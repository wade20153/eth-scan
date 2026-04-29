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
	"eth-scan/pkg/utils"
	"eth-scan/repository"
)

const ethTransferGasLimit = int64(21_000)   // ETH 标准转账固定 Gas
const erc20TransferGasLimit = int64(80_000) // ERC20 transfer 预估 Gas（含 buffer）
const confirmPollInterval = 5 * time.Second
const confirmTimeout = 5 * time.Minute // 等待 gas 到账最多 5 分钟（防止测试网拥堵超时）

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
		// 先检查余额：若已为 0 说明被之前的合并归集打走了，直接记账关闭本笔
		if bal, err := wallet.GetERC20Balance(contractAddr, record.ToAddress); err == nil && bal.Sign() == 0 {
			log.Printf("[归集] %s 余额已为 0（已被合并归集），直接记账 txHash=%s userID=%d",
				record.TokenType, record.TxHash, record.UserID)
			standardAmount := utils.ToStandardUnit(record.Value, record.TokenType)
			_ = repository.AddUserBalance(record.UserID, record.TokenType, standardAmount)
			_ = repository.UpdateTransactionStatus(record.TxHash, 2)
			log.Printf("[到账] userID=%d token=%s amount=%s", record.UserID, record.TokenType, standardAmount)
			return
		}
		sweepOK = s.sweepERC20(ctx, record, privKeyHex, contractAddr, chainID)
	}

	if !sweepOK {
		return
	}

	// ★ 归集成功：链上原始值换算为标准单位后存入余额表
	// USDT 600000000 → "600.000000"，ETH 1e18 → "1.000000000000000000"
	standardAmount := utils.ToStandardUnit(record.Value, record.TokenType)
	if err := repository.AddUserBalance(record.UserID, record.TokenType, standardAmount); err != nil {
		log.Printf("[归集] 更新用户余额失败 userID=%d token=%s: %v", record.UserID, record.TokenType, err)
		return
	}

	// ★ 交易状态改为 2=已到账
	if err := repository.UpdateTransactionStatus(record.TxHash, 2); err != nil {
		log.Printf("[归集] 更新交易状态失败 txHash=%s: %v", record.TxHash, err)
		return
	}

	log.Printf("[到账] userID=%d token=%s amount=%s", record.UserID, record.TokenType, standardAmount)
}

// sweepETH 归集 ETH，返回是否成功
func (s *Sweeper) sweepETH(ctx context.Context, record *repository.Transaction, childPrivKey string, chainID *big.Int) bool {
	balance, err := wallet.GetEthBalance(record.ToAddress)
	if err != nil {
		log.Printf("[归集-ETH] 查询余额失败 addr=%s: %v", record.ToAddress, err)
		return false
	}

	// 动态计算实际 Gas 费：gasPrice × 21000 + 20% buffer，避免固定值导致小额充值无法归集
	gasPrice, err := ethclient.GetInstance().GetSuggestGasPrice(ctx)
	if err != nil {
		log.Printf("[归集-ETH] 获取 GasPrice 失败: %v", err)
		return false
	}
	gasCost := new(big.Int).Mul(gasPrice, big.NewInt(ethTransferGasLimit))
	reserve := new(big.Int).Mul(gasCost, big.NewInt(120))
	reserve.Div(reserve, big.NewInt(100))

	if balance.Cmp(reserve) <= 0 {
		log.Printf("[归集-ETH] 余额不足以归集 addr=%s balance=%s reserve=%s", record.ToAddress, balance.String(), reserve.String())
		return false
	}

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

	// 第一步：查询子地址 ETH 余额，不足则由热钱包补足
	// 补给目标 = suggestedGasPrice × gasLimit × 1.5，确保有足够 ETH 覆盖 ERC20 转账 gas
	gasPrice, err := ethclient.GetInstance().GetSuggestGasPrice(ctx)
	if err != nil {
		log.Printf("[归集-ERC20] 获取 GasPrice 失败: %v", err)
		return false
	}
	gasTarget := new(big.Int).Mul(gasPrice, big.NewInt(erc20TransferGasLimit))
	gasTarget.Mul(gasTarget, big.NewInt(3))
	gasTarget.Div(gasTarget, big.NewInt(2)) // × 1.5

	childETHBalance, _ := wallet.GetEthBalance(record.ToAddress)
	log.Printf("[归集-ERC20] 子地址 ETH 余额: %s Wei，Gas 目标: %s Wei", childETHBalance.String(), gasTarget.String())

	if childETHBalance.Cmp(gasTarget) < 0 {
		// 补到 gasTarget，让子地址有足够 ETH 完成 ERC20 转账
		supplyAmount := new(big.Int).Sub(gasTarget, childETHBalance)
		toAddr := common.HexToAddress(record.ToAddress)
		log.Printf("[归集-ERC20] ETH 不足，从热钱包补 %s Wei", supplyAmount.String())
		gasTxHash, err := wallet.SendETH(s.hotPrivKey, toAddr, supplyAmount, chainID)
		if err != nil {
			log.Printf("[归集-ERC20] 补 Gas 失败 addr=%s: %v", record.ToAddress, err)
			return false
		}
		log.Printf("[归集-ERC20] 补 Gas txHash=%s，等待确认...", gasTxHash)
		if err := s.waitForETHBalance(ctx, record.ToAddress, gasTarget); err != nil {
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

// waitForETHBalance 轮询等待子地址 ETH 余额到达目标值，确认到账后再执行 ERC20 转账
func (s *Sweeper) waitForETHBalance(ctx context.Context, addr string, target *big.Int) error {
	deadline := time.Now().Add(confirmTimeout)
	log.Printf("[归集-ERC20] 等待 Gas 到账 addr=%s 目标=%s Wei（最多 %v）", addr, target.String(), confirmTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		balance, err := wallet.GetEthBalance(addr)
		if err == nil && balance.Cmp(target) >= 0 {
			log.Printf("[归集-ERC20] Gas 已到账 addr=%s balance=%s Wei，开始 ERC20 转账", addr, balance.String())
			return nil
		}
		time.Sleep(confirmPollInterval)
	}
	return fmt.Errorf("等待超时（%v）", confirmTimeout)
}

// StartRetryLoop 启动后台重试协程，定期扫描 status=0 且超过 minAge 的交易并重新归集
// 适用场景：首次归集因超时/网络抖动失败，确保资金最终一定归集到热钱包
func (s *Sweeper) StartRetryLoop(ctx context.Context, minAge time.Duration, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.retryPending(ctx, minAge)
			}
		}
	}()
}

// retryPending 查询所有未到账交易，逐笔重新触发归集
func (s *Sweeper) retryPending(ctx context.Context, minAge time.Duration) {
	txs, err := repository.GetPendingTransactions(minAge)
	if err != nil {
		log.Printf("[重试归集] 查询待处理交易失败: %v", err)
		return
	}
	if len(txs) == 0 {
		return
	}
	log.Printf("[重试归集] 发现 %d 笔未到账交易，开始重试...", len(txs))

	master, err := repository.GetDefaultMasterWallet()
	if err != nil {
		log.Printf("[重试归集] 获取主钱包失败: %v", err)
		return
	}

	for i := range txs {
		tx := &txs[i]
		ethAddr, err := repository.GetEthAddressByAddress(tx.ToAddress)
		if err != nil {
			log.Printf("[重试归集] 查询地址记录失败 addr=%s txHash=%s: %v", tx.ToAddress, tx.TxHash, err)
			continue
		}

		// 检测 pending nonce 积压：若子地址有多笔卡住的交易，重置到已确认 nonce
		// 这样新交易会替换最早那笔卡住的（nonce 最小），而不是追加到末尾
		childAddr := common.HexToAddress(tx.ToAddress)
		if confirmedNonce, err := ethclient.GetInstance().GetConfirmedNonce(ctx, childAddr); err == nil {
			if pendingNonce, _ := ethclient.GetInstance().GetTransactionCount(ctx, childAddr); pendingNonce > confirmedNonce {
				log.Printf("[重试归集] pending 积压 addr=%s confirmed=%d pending=%d，重置 nonce",
					tx.ToAddress, confirmedNonce, pendingNonce)
				wallet.ForceNonce(childAddr, confirmedNonce)
			}
		}

		log.Printf("[重试归集] 重试 txHash=%s token=%s userID=%d", tx.TxHash, tx.TokenType, tx.UserID)
		go s.Sweep(ctx, tx, ethAddr, master.MnemonicPlain)
	}
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
