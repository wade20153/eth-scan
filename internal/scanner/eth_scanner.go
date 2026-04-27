package scanner

import (
	"context"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"eth-scan/config"
	"eth-scan/pkg/ethclient"
	"eth-scan/repository"
)

const (
	chainName     = "ethereum"
	confirmBlocks = uint64(12)       // 以太坊主网建议 12 块确认，低于此高度的块视为不安全
	retryInterval = 3 * time.Second  // RPC 出错后的重试间隔
	catchUpSleep  = 12 * time.Second // 已追上最新安全块后的轮询等待时间
)

// transferEventSig ERC20 Transfer(address,address,uint256) 事件签名
var transferEventSig = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// EthScanner 以太坊区块扫描器
// 职责：轮询区块 → 解析 ETH 转账 + ERC20 Transfer 事件 → 识别充值 → 写入数据库
type EthScanner struct {
	cfg             *config.Config
	client          *ethclient.Eth
	watchAddressMap map[string]uint64 // 监控地址表：key=地址(小写), value=用户ID
	mapLock         sync.RWMutex      // 保护 watchAddressMap 的读写锁，支持运行时热更新
	contractMap     map[string]string // ERC20 合约表：key=合约地址(小写), value=代币符号
	erc20Addresses  []common.Address  // 供 FilterQuery 使用的合约地址切片
	currentBlockNum uint64            // 当前待扫描的区块高度
}

// NewEthScanner 创建扫块实例，注入配置与 RPC 客户端单例
func NewEthScanner(cfg *config.Config) *EthScanner {
	s := &EthScanner{
		cfg:             cfg,
		client:          ethclient.GetInstance(),
		watchAddressMap: make(map[string]uint64),
		contractMap:     make(map[string]string),
	}
	s.loadERC20Contracts()
	return s
}

// loadERC20Contracts 从配置加载需要监控的 ERC20 合约
func (s *EthScanner) loadERC20Contracts() {
	for _, c := range s.cfg.BlockChain.ERC20Contracts {
		addr := strings.ToLower(c.Address)
		s.contractMap[addr] = c.Symbol
		s.erc20Addresses = append(s.erc20Addresses, common.HexToAddress(c.Address))
	}
	log.Printf("ERC20 监控合约加载完成，共 %d 个: %v", len(s.contractMap), s.contractMap)
}

// Start 启动扫块主循环，收到 ctx.Done 信号后优雅退出
// 启动流程：
//  1. 从数据库加载监控地址到内存
//  2. 读取上次扫描高度；首次启动时以链上当前高度初始化
//  3. 进入循环，每轮调用 tick 处理一个区块（ETH + ERC20）
func (s *EthScanner) Start(ctx context.Context) {
	if err := s.FreshWatchAddressMap(); err != nil {
		log.Fatalf("初始化监控地址失败：%v", err)
	}

	localNum, err := s.GetLocalBlockNum()
	if err != nil {
		log.Fatalf("获取本地扫块高度失败: %v", err)
	}

	// 首次启动（数据库无记录），从链上当前高度开始，避免从创世块全量扫描
	if localNum == 0 {
		chainNum, err := s.client.GetLatestBlockNumber(ctx)
		if err != nil {
			log.Fatalf("获取链上最新高度失败: %v", err)
		}
		if err := repository.SaveScanHeight(chainName, chainNum); err != nil {
			log.Fatalf("初始化扫块高度失败: %v", err)
		}
		localNum = chainNum
		log.Printf("首次启动，初始化扫块高度为当前链高: %d", localNum)
	}

	s.currentBlockNum = localNum
	log.Printf("以太坊扫块服务启动，从高度 [%d] 开始...", s.currentBlockNum)

	for {
		select {
		case <-ctx.Done():
			log.Println("收到停止信号，优雅退出扫块服务...")
			return
		default:
			if err := s.tick(ctx); err != nil {
				log.Printf("扫块异常: %v，%v 后重试", err, retryInterval)
				time.Sleep(retryInterval)
			}
		}
	}
}

// tick 处理一个区块的完整流程：
//  1. 获取链上最新安全高度（latestBlock - confirmBlocks）
//  2. 若当前高度尚未到达安全高度则等待
//  3. 拉取区块数据，扫描 ETH 原生转账
//  4. 通过 GetLogs 扫描 ERC20 Transfer 事件
//  5. 更新数据库扫块进度，高度自增
func (s *EthScanner) tick(ctx context.Context) error {
	latestBlockNum, err := s.client.GetLatestBlockNumber(ctx)
	if err != nil {
		return err
	}

	// 只扫 confirmBlocks 之前的块，确保链上不会因重组而回滚
	safeBlockNum := latestBlockNum - confirmBlocks
	if s.currentBlockNum > safeBlockNum {
		// 已追上最新安全块，等待新块产生
		time.Sleep(catchUpSleep)
		return nil
	}

	// --- ETH 原生转账扫描 ---
	block, err := s.client.GetBlockByNumber(ctx, s.currentBlockNum)
	if err != nil {
		return err
	}
	s.processETHBlock(block)

	// --- ERC20 Transfer 事件扫描 ---
	if len(s.erc20Addresses) > 0 {
		if err := s.processERC20Logs(ctx, s.currentBlockNum); err != nil {
			log.Printf("ERC20 日志扫描失败 block=%d: %v", s.currentBlockNum, err)
		}
	}

	// 持久化当前扫块进度，重启后从此高度续扫
	if err := repository.UpdateScanHeight(chainName, s.currentBlockNum); err != nil {
		log.Printf("更新扫块高度失败: %v", err)
	}
	// 当前区块高度+1
	s.currentBlockNum++
	return nil
}

// processETHBlock 遍历区块内所有交易，匹配监控地址的 ETH 原生转账
func (s *EthScanner) processETHBlock(block *types.Block) {
	txs := block.Transactions()
	if len(txs) == 0 {
		return
	}
	for _, tx := range txs {
		if tx.To() == nil {
			continue // 合约创建交易无接收地址，跳过
		}
		toAddr := strings.ToLower(tx.To().Hex())
		s.mapLock.RLock()
		userID, exists := s.watchAddressMap[toAddr]
		s.mapLock.RUnlock()
		if !exists {
			continue
		}

		// 解析发送方地址
		signer := types.LatestSignerForChainID(tx.ChainId())
		from, err := types.Sender(signer, tx)
		if err != nil {
			log.Printf("解析 From 地址失败 txHash=%s: %v", tx.Hash().Hex(), err)
			continue
		}
		// 保存充值方式到数据库
		s.saveTransaction(&repository.Transaction{
			UserID:      userID,
			TxHash:      tx.Hash().Hex(),
			FromAddress: strings.ToLower(from.Hex()),
			ToAddress:   toAddr,
			Value:       tx.Value().String(), // 单位 Wei，字符串存储防精度丢失
			TokenType:   "ETH",
			BlockNumber: block.NumberU64(),
			BlockTime:   time.Unix(int64(block.Time()), 0),
			Status:      0, // 0=待确认，12块后可升级为已确认
		})
	}
}

// processERC20Logs 解析 USDT / USDC 等 ERC20 代币的充值
//
// 原理：ERC20 转账不像 ETH 直接看 tx.To()，而是由合约发出 Transfer 事件 Log。
// 每条 Log 结构如下：
//
//	topic[0] = Transfer 事件签名（固定值，用于过滤）
//	topic[1] = 付款方地址（from）        ← 谁转来的
//	topic[2] = 收款方地址（to）          ← ★ 关键：判断是否是我们的用户地址
//	data     = 转账金额（uint256，Wei级精度）
//
// 通过 eth_getLogs 一次拿到整块内所有 USDT/USDC Transfer 日志，效率远高于逐笔拉 Receipt
func (s *EthScanner) processERC20Logs(ctx context.Context, blockNum uint64) error {
	// ★ 第一步：构造过滤条件
	// Addresses = config.yaml 里配置的 USDT/USDC 合约地址，只看这些合约发出的事件
	// Topics[0] = Transfer 事件签名，只看转账事件，忽略合约的其他事件
	query := ethereum.FilterQuery{
		FromBlock: big.NewInt(int64(blockNum)),
		ToBlock:   big.NewInt(int64(blockNum)),
		Addresses: s.erc20Addresses, // 过滤：只看 USDT / USDC 合约
		Topics: [][]common.Hash{
			{transferEventSig}, // 过滤：只看 Transfer 事件
		},
	}

	// ★ 第二步：调用 eth_getLogs 拿到本块内所有符合条件的 Transfer 日志
	logs, err := s.client.GetLogs(ctx, query)
	if err != nil {
		return err
	}

	for _, vlog := range logs {
		if len(vlog.Topics) < 3 {
			continue
		}

		// ★ 第三步：从 topic[2] 解析收款地址（to）
		// topic 是 32 字节，地址只有 20 字节，前 12 字节是补零，HexToAddress 自动取后 20 字节
		toAddr := strings.ToLower(common.HexToAddress(vlog.Topics[2].Hex()).Hex())

		// ★ 第四步：判断收款地址是否是我们系统的用户地址
		s.mapLock.RLock()
		userID, exists := s.watchAddressMap[toAddr]
		s.mapLock.RUnlock()
		if !exists {
			continue // 不是我们的用户，跳过
		}

		// ★ 第五步：解析付款地址（from）和金额
		fromAddr := strings.ToLower(common.HexToAddress(vlog.Topics[1].Hex()).Hex())
		amount := new(big.Int).SetBytes(vlog.Data) // data = 金额，大端序 uint256

		// ★ 第六步：从合约地址反查代币符号（USDT 或 USDC）
		// vlog.Address 是合约地址，contractMap 存的是 合约地址 -> 符号 的映射
		symbol := s.contractMap[strings.ToLower(vlog.Address.Hex())]

		// ★ 第七步：写入充值记录
		// Value 存原始最小单位：USDT/USDC 精度均为 6 位，即 1 USDT = 1_000_000
		s.saveTransaction(&repository.Transaction{
			UserID:      userID,
			TxHash:      vlog.TxHash.Hex(),
			FromAddress: fromAddr,
			ToAddress:   toAddr,
			Value:       amount.String(), // 原始精度，如 "1000000" = 1 USDT/USDC
			TokenType:   symbol,          // "USDT" 或 "USDC"
			BlockNumber: blockNum,
			BlockTime:   time.Now(),
			Status:      0,
		})
	}
	return nil
}

// saveTransaction 幂等写入充值记录，tx_hash 已存在则跳过
func (s *EthScanner) saveTransaction(record *repository.Transaction) {
	created, err := repository.CreateTransactionIfNotExists(record)
	if err != nil {
		log.Printf("写入充值记录失败 txHash=%s: %v", record.TxHash, err)
		return
	}
	if !created {
		return // tx_hash 已存在，幂等跳过
	}
	log.Printf("命中充值! token=%s 用户ID=%d Hash=%s 金额=%s",
		record.TokenType, record.UserID, record.TxHash, record.Value)
}

// FreshWatchAddressMap 从数据库全量加载 status=1 的监控地址到内存
// 服务启动时调用一次；如需热更新可在运行时再次调用
func (s *EthScanner) FreshWatchAddressMap() error {
	addrs, err := repository.LoadActiveAddresses()
	if err != nil {
		return err
	}
	s.mapLock.Lock()
	defer s.mapLock.Unlock()
	s.watchAddressMap = make(map[string]uint64, len(addrs))
	for _, a := range addrs {
		s.watchAddressMap[strings.ToLower(a.Address)] = a.UserID
	}
	log.Printf("监控地址加载完成，共 %d 个", len(s.watchAddressMap))
	return nil
}

// AddWatchAddress 运行时动态新增监控地址，分配地址给用户时调用
func (s *EthScanner) AddWatchAddress(address string, userID uint64) {
	s.mapLock.Lock()
	s.watchAddressMap[strings.ToLower(address)] = userID
	s.mapLock.Unlock()
}

// RemoveWatchAddress 运行时移除监控地址，地址禁用时调用
func (s *EthScanner) RemoveWatchAddress(address string) {
	s.mapLock.Lock()
	delete(s.watchAddressMap, strings.ToLower(address))
	s.mapLock.Unlock()
}

// GetLocalBlockNum 从数据库读取上次扫描高度，首次返回 0
func (s *EthScanner) GetLocalBlockNum() (uint64, error) {
	return repository.GetScanHeight(chainName)
}
