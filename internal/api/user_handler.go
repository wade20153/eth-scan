package api

import (
	"eth-scan/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	svc service.IUserService
}

func NewUserHandler(svc service.IUserService) *UserHandler {
	return &UserHandler{
		svc: svc,
	}
}

// GetUserInfo godoc
// @Summary      查询用户信息
// @Description  查询用户基本信息及各币种余额
// @Tags         user
// @Produce      json
// @Param        user_id  path  int  true  "用户ID"
// @Success      200  {object}  map[string]interface{}
// @Failure      400  {object}  map[string]interface{}
// @Router       /user/{user_id} [get]
func (h *UserHandler) GetUserInfo(c *gin.Context) {
	userID, err := strconv.ParseUint(c.Param("user_id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 1, "message": "user_id 无效"})

	}
	info, err := h.svc.GetUserInfo(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 1, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "success", "data": info})
}
