package main

import (
	"eth-scan/config"
	"eth-scan/pkg/ethclient"
	"log"
)

func main() {
	// 第一步：加载配置
	// 这里的路径根据你实际存放 config.yaml 的位置决定
	cfg := config.LoadConfig()
	// 第二步：初始化以太坊客户端连接池
	// 此时 cfg.BlockChain.EthApiUrl 已经有值了
	ethclient.Connect(cfg.BlockChain.EthApiUrl)
	// 第三步：验证连接是否正常
	client := ethclient.GetInstance().GetClient()
	if client != nil {
		log.Println("🚀 以太坊扫描程序启动成功，准备进入扫块逻辑...")
	}

}
