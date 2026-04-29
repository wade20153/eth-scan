package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"eth-scan/config"
	"eth-scan/internal/wallet"
	"eth-scan/repository"
)

// CreateHDWallet GET /api/wallet/create
// 生成 BIP-44 HD 钱包，助记词 AES-GCM 加密后入库，返回钱包 ID 与主地址（不返回助记词）
func CreateHDWallet(c *gin.Context) {
	cfg := config.GetConfig()

	mnemonic, err := wallet.GenerateMnemonic()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "生成助记词失败: " + err.Error()})
		return
	}

	encrypted, err := wallet.EncryptMnemonic(mnemonic, cfg.Security.MnemonicSalt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "加密助记词失败: " + err.Error()})
		return
	}

	// index=0 的地址作为主钱包标识地址
	address, err := wallet.DeriveAddress(mnemonic, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "派生地址失败: " + err.Error()})
		return
	}

	master := &repository.MasterWallet{
		MnemonicEncrypted: encrypted,
		MnemonicPlain:     mnemonic,
		Address:           address.Hex(),
	}
	if err := repository.CreateMasterWallet(master); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "保存钱包失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data": gin.H{
			"wallet_id": master.ID,
			"address":   address.Hex(),
		},
	})
}

// BatchCreateUsers GET /api/wallet/batch-users
// 从系统默认 HD 钱包派生 10 个子地址，各创建一个用户并绑定地址
func BatchCreateUsers(c *gin.Context) {
	cfg := config.GetConfig()

	master, err := repository.GetDefaultMasterWallet()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "未找到 HD 钱包，请先调用 /api/wallet/create: " + err.Error()})
		return
	}

	walletID := uint64(master.ID)

	mnemonic, err := wallet.DecryptMnemonic(master.MnemonicEncrypted, cfg.Security.MnemonicSalt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "解密助记词失败: " + err.Error()})
		return
	}

	// 原子获取下一批派生起始 index，防止并发重复
	const batchSize = 10
	startIndex, err := repository.IncrDeriveCount(walletID, batchSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "获取派生索引失败: " + err.Error()})
		return
	}

	type userResult struct {
		UserID   uint   `json:"user_id"`
		Username string `json:"username"`
		Address  string `json:"address"`
		Index    uint32 `json:"derive_index"`
	}

	now := time.Now()
	results := make([]userResult, 0, batchSize)
	addrs := make([]*repository.EthAddress, 0, batchSize)

	for i := 0; i < batchSize; i++ {
		idx := startIndex + uint32(i)

		address, err := wallet.DeriveAddress(mnemonic, idx)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": fmt.Sprintf("派生地址失败 index=%d: %s", idx, err.Error())})
			return
		}

		username := fmt.Sprintf("user_%d_%d", now.UnixMilli(), idx)
		user := &repository.User{Username: username}
		if err := repository.CreateUser(user); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "创建用户失败: " + err.Error()})
			return
		}

		addrs = append(addrs, &repository.EthAddress{
			MasterWalletID: walletID,
			UserID:         uint64(user.ID),
			Address:        address.Hex(),
			DeriveIndex:    idx,
			Status:         1,
			AssignedAt:     &now,
		})

		results = append(results, userResult{
			UserID:   user.ID,
			Username: username,
			Address:  address.Hex(),
			Index:    idx,
		})
	}

	if err := repository.BatchCreateEthAddresses(addrs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": "保存地址失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    results,
	})
}
