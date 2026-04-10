package scanner

import (
	"eth-scan/config"
	"eth-scan/pkg/ethclient"
	"sync"
)

type EthScanner struct {
	cfg               *config.Config
	client            *ethclient.Eth
	monitorAddressMap map[string]uint64 // key: 地址(小写), value: 用户ID
	mapLock           sync.RWMutex      // 如果运行中会动态增加地址，需要加锁
}

func NewEthScanner(cfg *config.Config) *EthScanner {
	return &EthScanner{cfg: cfg}
}

func (s *EthScanner) Start() {
	// 获取客户端实例
	// 获得需要扫描的高度
	// 获取链上最新高度
	// 判断，如果当前高度《 链上高度
	// 获取当前区块的交易信息，处理

	// 当前区块高度+1，并存入数据库
	//

}
