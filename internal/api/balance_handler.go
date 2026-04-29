package api

import (
	"math/big"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"eth-scan/repository"
)

// tokenDecimals 各代币最小单位精度
// ETH: 18位(1 ETH = 10^18 Wei)，USDT/USDC: 6位(1 USDT = 10^6)
var tokenDecimals = map[string]int64{
	"ETH":  18,
	"USDT": 6,
	"USDC": 6,
}

// GetUserBalances GET /api/balance/:user_id
// 查询用户所有代币余额，并转换为人民币估值
func GetUserBalances(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "user_id 无效"})
		return
	}

	balances, err := repository.GetUserBalances(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}

	rates, err := repository.GetAllExchangeRates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	rateMap := make(map[string]string, len(rates))
	for _, r := range rates {
		rateMap[r.TokenType] = r.CNYRate
	}

	type balanceItem struct {
		TokenType  string `json:"token_type"`
		Balance    string `json:"balance"`     // 最小单位原始值
		BalanceStr string `json:"balance_str"` // 标准单位，如 "1.500000 ETH"
		CNYRate    string `json:"cny_rate"`    // 1个代币 = 多少人民币
		CNYValue   string `json:"cny_value"`   // 折合人民币
	}

	items := make([]balanceItem, 0, len(balances))
	for _, b := range balances {
		item := balanceItem{
			TokenType: b.TokenType,
			Balance:   b.Balance,
			CNYRate:   rateMap[b.TokenType],
		}

		// 转换为标准单位和人民币估值
		item.BalanceStr, item.CNYValue = convertBalance(b.Balance, b.TokenType, rateMap[b.TokenType])
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    items,
	})
}

// GetExchangeRates GET /api/exchange-rate
// 查询所有代币汇率
func GetExchangeRates(c *gin.Context) {
	rates, err := repository.GetAllExchangeRates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": rates})
}

// UpdateExchangeRate POST /api/exchange-rate
// 新增或更新汇率
// Body: { "token_type": "ETH", "cny_rate": "18000.00" }
func UpdateExchangeRate(c *gin.Context) {
	var req struct {
		TokenType string `json:"token_type" binding:"required"`
		CNYRate   string `json:"cny_rate" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "参数错误: " + err.Error()})
		return
	}

	// 校验汇率是否为合法正数
	rate, ok := new(big.Float).SetString(req.CNYRate)
	if !ok || rate.Sign() <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "cny_rate 必须为正数"})
		return
	}

	if err := repository.UpsertExchangeRate(req.TokenType, req.CNYRate, "admin"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// convertBalance 将最小单位余额转为标准单位字符串和人民币估值
func convertBalance(rawBalance, tokenType, cnyRate string) (balanceStr, cnyValue string) {
	decimals, ok := tokenDecimals[tokenType]
	if !ok {
		decimals = 18
	}

	raw, ok := new(big.Int).SetString(rawBalance, 10)
	if !ok || raw.Sign() == 0 {
		return "0", "0"
	}

	// 除以 10^decimals 得到标准单位
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	standard := new(big.Float).Quo(
		new(big.Float).SetInt(raw),
		new(big.Float).SetInt(divisor),
	)
	balanceStr = standard.Text('f', int(decimals)) + " " + tokenType

	// 乘以汇率得到人民币
	if cnyRate == "" {
		return balanceStr, "0"
	}
	rate, ok := new(big.Float).SetString(cnyRate)
	if !ok {
		return balanceStr, "0"
	}
	cny := new(big.Float).Mul(standard, rate)
	cnyValue = cny.Text('f', 2) + " CNY"
	return
}
