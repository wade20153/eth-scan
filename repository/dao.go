package repository

import (
	"errors"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var db *gorm.DB

// InitDB 初始化数据库连接（不执行迁移）
func InitDB(dsn string) error {
	var err error
	db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	return err
}

// Migrate 执行表结构迁移，仅在明确需要时调用（如首次部署或 --migrate 参数）
func Migrate() error {
	return db.AutoMigrate(
		&User{},
		&MasterWallet{},
		&EthAddress{},
		&Transaction{},
		&UserBalance{},
		&ExchangeRate{},
		&ScanHeight{},
	)
}

// GetDB 获取全局 db 实例
func GetDB() *gorm.DB {
	return db
}

// ---- User ----

func CreateUser(u *User) error {
	return db.Create(u).Error
}

// ---- MasterWallet ----

func CreateMasterWallet(w *MasterWallet) error {
	return db.Create(w).Error
}

// GetAllMasterWallets 获取全部主钱包（用于回填）
func GetAllMasterWallets() ([]MasterWallet, error) {
	var wallets []MasterWallet
	err := db.Find(&wallets).Error
	return wallets, err
}

// UpdateMnemonicPlain 回填明文助记词
func UpdateMnemonicPlain(id uint, plain string) error {
	return db.Model(&MasterWallet{}).Where("id = ?", id).Update("mnemonic_plain", plain).Error
}

// GetDefaultMasterWallet 取 id 最小的主钱包作为系统默认钱包
func GetDefaultMasterWallet() (*MasterWallet, error) {
	var w MasterWallet
	err := db.First(&w).Error
	return &w, err
}

func GetMasterWalletByID(id uint64) (*MasterWallet, error) {
	var w MasterWallet
	err := db.First(&w, id).Error
	return &w, err
}

// IncrDeriveCount 原子性地将 derive_count 加 n，返回派生前的旧值（即下一批起始 index）
func IncrDeriveCount(masterID uint64, n uint32) (uint32, error) {
	var w MasterWallet
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&w, masterID).Error; err != nil {
			return err
		}
		return tx.Model(&w).Update("derive_count", w.DeriveCount+n).Error
	})
	return w.DeriveCount, err
}

// ---- EthAddress ----

func BatchCreateEthAddresses(addrs []*EthAddress) error {
	return db.CreateInBatches(addrs, 100).Error
}

// AssignAddress 从空闲池取一个地址分配给用户
func AssignAddress(userID uint64) (*EthAddress, error) {
	var addr EthAddress
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("status = 0").First(&addr).Error; err != nil {
			return errors.New("地址池已耗尽，请先派生更多地址")
		}
		return tx.Model(&addr).Updates(map[string]interface{}{
			"user_id": userID,
			"status":  1,
		}).Error
	})
	return &addr, err
}

// LoadActiveAddresses 加载所有 status=1 的地址（供扫块引擎初始化）
func LoadActiveAddresses() ([]EthAddress, error) {
	var addrs []EthAddress
	err := db.Where("status = 1").Find(&addrs).Error
	return addrs, err
}

func DisableAddress(address string) error {
	return db.Model(&EthAddress{}).Where("address = ?", address).Update("status", 2).Error
}

// GetEthAddressByAddress 按地址字符串查询记录（归集时用于获取派生索引）
// 使用 LOWER() 避免 EIP-55 混合大小写与全小写不匹配问题
func GetEthAddressByAddress(address string) (*EthAddress, error) {
	var addr EthAddress
	err := db.Where("LOWER(address) = ?", strings.ToLower(address)).First(&addr).Error
	return &addr, err
}

// ---- Transaction ----

// CreateTransactionIfNotExists 幂等写入，tx_hash 已存在则跳过
func CreateTransactionIfNotExists(t *Transaction) (bool, error) {
	result := db.Where(Transaction{TxHash: t.TxHash}).FirstOrCreate(t)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func UpdateTransactionStatus(txHash string, status int8) error {
	return db.Model(&Transaction{}).Where("tx_hash = ?", txHash).Update("status", status).Error
}

// GetPendingTransactions 查询超过 minAge 时间仍未到账（status=0）的交易，用于重试归集
func GetPendingTransactions(minAge time.Duration) ([]Transaction, error) {
	var txs []Transaction
	threshold := time.Now().Add(-minAge)
	err := db.Where("status = 0 AND created_at < ?", threshold).Find(&txs).Error
	return txs, err
}

// ---- ScanHeight ----

func GetScanHeight(chain string) (uint64, error) {
	var s ScanHeight
	err := db.Where("chain = ?", chain).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	return s.BlockNumber, err
}

func SaveScanHeight(chain string, blockNumber uint64) error {
	return db.Where(ScanHeight{Chain: chain}).
		Assign(ScanHeight{BlockNumber: blockNumber}).
		FirstOrCreate(&ScanHeight{}).Error
}

func UpdateScanHeight(chain string, blockNumber uint64) error {
	return db.Model(&ScanHeight{}).
		Where("chain = ?", chain).
		Update("block_number", blockNumber).Error
}

// ---- UserBalance ----

// GetOrCreateUserBalance 查询用户余额，不存在则初始化一条 Balance="0" 的记录
func GetOrCreateUserBalance(userID uint64, tokenType string) (*UserBalance, error) {
	var b UserBalance
	result := db.Where(UserBalance{UserID: userID, TokenType: tokenType}).
		Attrs(UserBalance{Balance: "0"}).
		FirstOrCreate(&b)
	return &b, result.Error
}

// AddUserBalance 原子性地给用户余额加上 amount（字符串形式的最小单位数值）
func AddUserBalance(userID uint64, tokenType, amount string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var b UserBalance
		if err := tx.Where(UserBalance{UserID: userID, TokenType: tokenType}).
			Attrs(UserBalance{Balance: "0"}).
			FirstOrCreate(&b).Error; err != nil {
			return err
		}
		// 用 SQL 直接做字符串转数值加法，避免 Go 层 big.Int 与数据库并发竞争
		return tx.Model(&b).Update("balance",
			gorm.Expr("CAST(balance AS DECIMAL(65,18)) + CAST(? AS DECIMAL(65,18))", amount),
		).Error
	})
}

// GetUserBalances 查询用户所有代币余额
func GetUserBalances(userID uint64) ([]UserBalance, error) {
	var balances []UserBalance
	err := db.Where("user_id = ?", userID).Find(&balances).Error
	return balances, err
}

// ---- ExchangeRate ----

// UpsertExchangeRate 新增或更新汇率，存在则覆盖
func UpsertExchangeRate(tokenType, cnyRate, updatedBy string) error {
	return db.Where(ExchangeRate{TokenType: tokenType}).
		Assign(ExchangeRate{CNYRate: cnyRate, UpdatedBy: updatedBy}).
		FirstOrCreate(&ExchangeRate{}).Error
}

// GetExchangeRate 查询单个代币汇率，不存在返回 ErrRecordNotFound
func GetExchangeRate(tokenType string) (*ExchangeRate, error) {
	var r ExchangeRate
	err := db.Where("token_type = ?", tokenType).First(&r).Error
	return &r, err
}

// GetAllExchangeRates 查询所有汇率配置
func GetAllExchangeRates() ([]ExchangeRate, error) {
	var rates []ExchangeRate
	err := db.Find(&rates).Error
	return rates, err
}
