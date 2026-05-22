package response

import "eth-scan/repository"

type UserInfoResult struct {
	UserID   uint64
	Username string
	Balance  []repository.UserBalance
}
