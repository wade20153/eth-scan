package scanner

import (
	"context"
	"eth-scan/config"
	"eth-scan/pkg/ethclient"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/core/types"
)

type EthScanner struct {
	cfg             *config.Config
	client          *ethclient.Eth
	watchAddressMap map[string]uint64 // key: 地址(小写), value: 用户ID
	mapLock         sync.RWMutex      // 如果运行中会动态增加地址，需要加锁
	currentBlockNum uint64            // 内存中维护当前扫描高度
}

func NewEthScanner(cfg *config.Config) *EthScanner {
	return &EthScanner{cfg: cfg}
}

func (s *EthScanner) Start(ctx context.Context) {
	// 1. 初始化/刷新监控地址池 (从数据库加载)
	err := s.FreshWatchAddressMap()
	if err != nil {
		log.Fatalf("初始化监控地址失败：%v", err)
		return
	}
	log.Printf("以太坊扫快服务已经启动...")
	// 获得需要扫描的高度
	localNum, err := s.GetLocalBlockNum()
	s.currentBlockNum = localNum
	log.Printf("以太坊扫块服务启动，从高度 [%d] 开始...", s.currentBlockNum)
	for {
		select {
		case <-ctx.Done():
			log.Println("📥 收到停止信号，优雅退出扫块服务...")
			return
		default:

			if err != nil {
				log.Printf("⚠️ 获取本地高度失败: %v, 3秒后重试", err)
				time.Sleep(3 * time.Second)
				continue
			}
			// 获取链上最新高度
			latestBlockNum, err := s.client.GetLatestBlockNumber(ctx)
			if err != nil {
				log.Fatalf("获取最新高度失败: %v", err)
			}
			// 以太坊通常建议 12 块确认
			safeBlockNum := latestBlockNum - 12
			// 获取链上最新高度
			if s.currentBlockNum > safeBlockNum {
				// 判断，如果当前高度《 链上高度
				time.Sleep(12)
			}
			// 3. 抓取区块数据
			block, err := s.client.GetBlockByNumber(ctx, s.currentBlockNum)
			if err != nil {
				log.Printf("获取区块 [%d] 失败； %v,充值中", err)
			}
			// 4. 处理区块内的交易
			s.processBlock(block)
			// 5. 高度推进并持久化
			s.updateLocalBlockNum(s.currentBlockNum)
			s.currentBlockNum++
		}
	}

}

func (s *EthScanner) FreshWatchAddressMap() error {
	// 从 repository 层查询所有生成的地址
	return nil
}

// GetLocalBlockNum 获取当前高度，启动时候使用
func (s *EthScanner) GetLocalBlockNum() (uint64, error) {
	return 0, nil
}

func (s *EthScanner) processBlock(block *types.Block) {
	transaction := block.Transactions()
	if len(transaction) == 0 {
		return
	}
	for _, tx := range transaction {
		// 只需要处理转账交易，即有 To 地址且 To 在我们的监控列表中
		if tx.To() == nil {
			continue
		}
		toAddress := strings.ToLower(tx.To().Hex())
		s.mapLock.RLock()
		userId, exists := s.watchAddressMap[toAddress]
		if exists {
			s.handleMatchedTx(userId, tx, block)
		}

	}

}

func (s *EthScanner) updateLocalBlockNum(num uint64) {

}

// handleMatchedTx 命中后的业务处理，建议异步或进入 MQ
func (s *EthScanner) handleMatchedTx(userId uint64, tx *types.Transaction, block *types.Block) {
	value := tx.Value()
	txHash := tx.Hash().Hex()
	log.Printf("🔥 命中充值交易! 用户ID: %d, Hash: %s, 金额: %s Wei", userId, txHash, value.String())
	// TODO: 调用 Repository 写入充值流水表记录
	// 建议在这里判断 tx.Status，确保交易在链上执行成功 (需要额外调用 GetTransactionReceipt)
}
