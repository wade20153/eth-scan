package utils

import (
	"math/big"
)

var (
	weiPerEther = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil) // 10^18
	weiPerGwei  = new(big.Int).Exp(big.NewInt(10), big.NewInt(9), nil)  // 10^9
)

// tokenDecimalsMap 各代币精度
var tokenDecimalsMap = map[string]int64{
	"ETH":  18,
	"USDT": 6,
	"USDC": 6,
}

// ToStandardUnit 将链上原始最小单位转为标准单位字符串
// 例：ETH "1000000000000000000" → "1.000000000000000000"
//
//	USDT "600000000" → "600.000000"
func ToStandardUnit(rawValue, tokenType string) string {
	decimals, ok := tokenDecimalsMap[tokenType]
	if !ok {
		decimals = 18
	}
	raw, ok := new(big.Int).SetString(rawValue, 10)
	if !ok || raw.Sign() == 0 {
		return "0"
	}
	divisor := new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil)
	// 整数部分
	quotient := new(big.Int).Quo(raw, divisor)
	// 小数部分
	remainder := new(big.Int).Rem(raw, divisor)
	scale := new(big.Int).Mul(remainder, new(big.Int).Exp(big.NewInt(10), big.NewInt(decimals), nil))
	fraction := new(big.Int).Quo(scale, divisor)
	return quotient.String() + "." + zeroPad(fraction.String(), int(decimals))
}

// WeiToEtherString 将 Wei 转换为可读的 ETH 字符串（保留 6 位小数）
// 仅用于日志展示，业务计算禁止使用此函数
func WeiToEtherString(wei *big.Int) string {
	if wei == nil {
		return "0"
	}
	// 整数部分
	quotient := new(big.Int).Quo(wei, weiPerEther)
	// 余数（小数部分）= remainder / 10^18，保留 6 位
	remainder := new(big.Int).Rem(wei, weiPerEther)
	// 余数乘以 10^6 再除以 10^18 得到 6 位小数
	scale := new(big.Int).Mul(remainder, big.NewInt(1_000_000))
	fraction := new(big.Int).Quo(scale, weiPerEther)
	return quotient.String() + "." + zeroPad(fraction.String(), 6) + " ETH"
}

// EtherToWei 将 ETH 字符串（如 "1.5"）转换为 Wei
// 注意：仅支持整数 ETH，带小数请使用 decimal 库
func EtherToWei(etherInt int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(etherInt), weiPerEther)
}

// GweiToWei 将 Gwei 转换为 Wei
func GweiToWei(gwei int64) *big.Int {
	return new(big.Int).Mul(big.NewInt(gwei), weiPerGwei)
}

// WeiToGwei 将 Wei 转换为 Gwei（整数，向下取整）
func WeiToGwei(wei *big.Int) *big.Int {
	return new(big.Int).Quo(wei, weiPerGwei)
}

// ParseWei 将字符串解析为 Wei big.Int
func ParseWei(s string) (*big.Int, bool) {
	n := new(big.Int)
	_, ok := n.SetString(s, 10)
	return n, ok
}

func zeroPad(s string, length int) string {
	for len(s) < length {
		s = "0" + s
	}
	return s
}
