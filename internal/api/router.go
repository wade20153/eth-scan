package api

import "github.com/gin-gonic/gin"

func NewRouter() *gin.Engine {
	r := gin.Default()

	api := r.Group("/api")
	{
		wallet := api.Group("/wallet")
		{
			wallet.GET("/create", CreateHDWallet)
			wallet.GET("/batch-users", BatchCreateUsers)
		}

		// 余额查询
		api.GET("/balance/:user_id", GetUserBalances)

		// 汇率管理
		rate := api.Group("/exchange-rate")
		{
			rate.GET("", GetExchangeRates)
			rate.POST("", UpdateExchangeRate)
		}
	}

	return r
}
