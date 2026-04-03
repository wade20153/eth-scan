package config

import (
	"fmt"
	"log"
	"sync"

	"github.com/spf13/viper"
)

// Config 对应你的业务配置需求
type Config struct {
	BlockChain struct {
		EthApiUrl          []string `mapstructure:"eth_api_url"`
		EthContractAddress string   `mapstructure:"eth_contract_address"`
		EthMinGasAmount    string   `mapstructure:"eth_min_gas_amount"`
		EthMinUsdtAmount   string   `mapstructure:"eth_min_usdt_amount"`
	} `mapstructure:"blockchain"`
}

var (
	conf *Config
	once sync.Once
)

// GetConfig 获取全局配置单例
func GetConfig() *Config {
	if conf == nil {
		log.Fatal("Config 尚未初始化，请先调用 LoadConfig")
	}
	return conf
}

// LoadConfig 从文件或环境变量加载配置
// config/config.go

// LoadConfig 加载配置
func LoadConfig() *Config {
	once.Do(func() {
		v := viper.New()

		// 1. 设置配置文件的名称（不需要后缀）
		v.SetConfigName("config")
		// 2. 设置配置文件类型
		v.SetConfigType("yaml")

		// 3. 关键点：添加搜索路径
		// 如果你在项目根目录运行，config.yaml 在 ./config/ 目录下
		v.AddConfigPath("./config")
		// 如果你在 config 目录下跑测试，他在当前目录 .
		v.AddConfigPath(".")

		// 允许环境变量覆盖
		v.AutomaticEnv()

		// 读取文件
		if err := v.ReadInConfig(); err != nil {
			panic(fmt.Errorf("读取配置文件失败: %w", err))
		}

		// 反序列化到结构体
		if err := v.Unmarshal(&conf); err != nil {
			panic(fmt.Errorf("配置解析失败: %w", err))
		}

		log.Printf("✅ 配置文件加载成功: %s", v.ConfigFileUsed())
	})
	return conf
}
