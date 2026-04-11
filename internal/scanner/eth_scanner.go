package scanner

import (
	"context"
	"eth-scan/config"
	"eth-scan/pkg/ethclient"
	"log"
	"sync"
	"time"
)

type EthScanner struct {
	cfg             *config.Config
	client          *ethclient.Eth
	watchAddressMap map[string]uint64 // key: 地址(小写), value: 用户ID
	mapLock         sync.RWMutex      // 如果运行中会动态增加地址，需要加锁
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

	for {
		select {
		case <-ctx.Done():
			log.Println("📥 收到停止信号，优雅退出扫块服务...")
			return
		default:
			// 获得需要扫描的高度
			currentBlockNum, err := s.GetLocalBlockNum()
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
			// 获取链上最新高度
			if currentBlockNum > latestBlockNum-12 {
				time.Sleep(12)
			}

		}
	}

	// 判断，如果当前高度《 链上高度
	// 获取当前区块的交易信息，处理

	// 当前区块高度+1，并存入数据库
	//

}

func (s *EthScanner) FreshWatchAddressMap() error {
	// 从 repository 层查询所有生成的地址
	return nil
}

// GetLocalBlockNum 获取当前高度，启动时候使用
func (s *EthScanner) GetLocalBlockNum() (uint64, error) {
	return 0, nil
}
