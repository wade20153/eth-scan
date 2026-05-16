package api

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "eth-scan/docs" // swag 生成的文档包
)

func NewRouter() *gin.Engine {
	r := gin.Default()

	// Swagger UI：访问 http://localhost:8080/swagger/index.html
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
