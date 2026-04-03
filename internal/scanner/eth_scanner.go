package scanner

import "eth-scan/config"

type EthScanner struct {
	cfg *config.Config
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
