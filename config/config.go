package config

import (
	"fmt"
	"log"
	"sync"

	"github.com/spf13/viper"
)

// ERC20Contract 单个 ERC20 合约配置
type ERC20Contract struct {
	Address string `mapstructure:"address"`
	Symbol  string `mapstructure:"symbol"`
}

type Config struct {
	Server struct {
		Port string `mapstructure:"port"`
	} `mapstructure:"server"`
	BlockChain struct {
		EthApiUrl        []string        `mapstructure:"eth_api_url"`
		ERC20Contracts   []ERC20Contract `mapstructure:"erc20_contracts"`
		EthMinGasAmount  string          `mapstructure:"eth_min_gas_amount"`
		EthMinUsdtAmount string          `mapstructure:"eth_min_usdt_amount"`
	} `mapstructure:"blockchain"`
	Database struct {
		DSN string `mapstructure:"dsn"`
	} `mapstructure:"database"`
	Security struct {
		MnemonicSalt string `mapstructure:"mnemonic_salt"`
	} `mapstructure:"security"`
	HotWallet struct {
		Address    string `mapstructure:"address"`     // 入金归集地址
		PrivateKey string `mapstructure:"private_key"` // 入金地址私钥（用于补 Gas 和签名）
	} `mapstructure:"hot_wallet"`
}

var (
	conf *Config
	once sync.Once
)

func GetConfig() *Config {
	if conf == nil {
		log.Fatal("Config 尚未初始化，请先调用 LoadConfig")
	}
	return conf
}

func LoadConfig() *Config {
	once.Do(func() {
		v := viper.New()
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./config")
		v.AddConfigPath(".")
		v.AutomaticEnv()
		v.SetDefault("server.port", "8080")

		if err := v.ReadInConfig(); err != nil {
			panic(fmt.Errorf("读取配置文件失败: %w", err))
		}
		if err := v.Unmarshal(&conf); err != nil {
			panic(fmt.Errorf("配置解析失败: %w", err))
		}
		log.Printf("✅ 配置文件加载成功: %s", v.ConfigFileUsed())
	})
	return conf
}
