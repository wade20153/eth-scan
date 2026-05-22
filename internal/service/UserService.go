package service

import (
	"context"
	"eth-scan/internal/param/response"
	"eth-scan/repository"
)

type (
	// IUserService 用户业务接口，handler 依赖此接口而非具体实现
	IUserService interface {
		GetUserInfo(ctx context.Context, UserID uint64) (*response.UserInfoResult, error)
	}
	// userService 不对外暴露，通过 NewUserService 构造
	userService struct{}
)

// NewUserService 返回接口，隐藏具体实现
func NewUserService() IUserService {
	return &userService{}
}

// GetUserInfo 查询用户基本信息及各币种余额
func (u *userService) GetUserInfo(ctx context.Context, UserID uint64) (*response.UserInfoResult, error) {
	user, err := repository.CetUserByID(UserID)
	if err != nil {
		return nil, err
	}
	s, err := repository.GetBalanceS(UserID)
	if err != nil {
		return nil, err
	}
	return &response.UserInfoResult{
		Username: user.Username,
		UserID:   UserID,
		Balance:  s,
	}, nil
}
