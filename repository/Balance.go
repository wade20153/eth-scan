package repository

func GetBalanceS(userId uint64) ([]UserBalance, error) {
	var userBalances []UserBalance
	err := db.Where("user_id = ?", userId).Find(&userBalances).Error
	return userBalances, err
}
