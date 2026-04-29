package repository

import (
	"time"

	"gorm.io/gorm"
)

// User 平台用户
type User struct {
	gorm.Model
	Username string `gorm:"uniqueIndex;size:64;not null"`
}

// MasterWallet 主地址（助记词根），所有子地址由此派生
type MasterWallet struct {
	gorm.Model
	MnemonicEncrypted string `gorm:"size:512;not null"` // AES-GCM 加密后的助记词
	MnemonicPlain     string `gorm:"size:256"`          // 明文助记词，仅供内部记录，生产环境注意数据库访问权限
	Address           string `gorm:"uniqueIndex;size:42;not null"`
	DeriveCount       uint32 `gorm:"default:0"` // 已派生子地址数量，用于续派
}

// EthAddress 子地址，与用户绑定，用于接收充值
type EthAddress struct {
	gorm.Model
	MasterWalletID uint64 `gorm:"not null;index"`
	UserID         uint64 `gorm:"index;default:0"` // 0 表示未分配
	Address        string `gorm:"uniqueIndex;size:42;not null"`
	DeriveIndex    uint32 `gorm:"not null"`
	Status         int8   `gorm:"default:0"` // 0=未使用 1=使用中 2=已禁用
	AssignedAt     *time.Time
}

// Transaction 链上充值流水
type Transaction struct {
	gorm.Model
	UserID      uint64 `gorm:"not null;index"`
	TxHash      string `gorm:"uniqueIndex;size:66;not null"` // 唯一索引防重入
	FromAddress string `gorm:"size:42"`
	ToAddress   string `gorm:"size:42;index"`
	Value       string `gorm:"size:80;not null"` // Wei，字符串存储防精度丢失
	TokenType   string `gorm:"size:20;not null"` // ETH / USDT / ...
	BlockNumber uint64
	BlockTime   time.Time
	Status      int8 `gorm:"default:0"` // 0=待确认 1=已确认 2=已到账
}

// UserBalance 用户余额表，按用户+代币类型分别记录
// Balance 使用字符串存储最小单位（Wei / USDT最小单位），禁止 float64
type UserBalance struct {
	gorm.Model
	UserID    uint64 `gorm:"uniqueIndex:idx_user_token;not null"`
	TokenType string `gorm:"uniqueIndex:idx_user_token;size:20;not null"` // ETH / USDT / USDC
	Balance   string `gorm:"size:80;not null;default:'0'"`                // 最小单位字符串
}

// ExchangeRate 汇率配置表，存储各代币对人民币(CNY)的汇率
// Rate 字符串存储，如 ETH="18000.00"，USDT="7.30"
// 精度用 big.Float 处理，禁止 float64 直接运算
type ExchangeRate struct {
	gorm.Model
	TokenType string `gorm:"uniqueIndex;size:20;not null"` // ETH / USDT / USDC
	CNYRate   string `gorm:"size:32;not null"`             // 1个代币 = 多少人民币
	UpdatedBy string `gorm:"size:64"`                      // 最后更新人/来源
}

// ScanHeight 扫块进度，服务重启后从此续扫
type ScanHeight struct {
	gorm.Model
	Chain       string `gorm:"uniqueIndex;size:32;not null"`
	BlockNumber uint64 `gorm:"not null"`
}
