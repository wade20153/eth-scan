package repository

func CetUserByID(userID uint64) (*User, error) {
	var u User
	err := db.First(&u, userID).Error
	return &u, err

}
func CetUserByIDSQL(userID uint64) (*User, error) {
	var u User
	// 查询结果
	row := db.Raw("SELECT * FROM users WHERE id = ?", userID)
	// 赋值
	err := row.Scan(&u).Error
	return &u, err

}
