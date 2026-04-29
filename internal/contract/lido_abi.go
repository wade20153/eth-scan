package contract

import (
	"github.com/ethereum/go-ethereum/common"
)

// Lido stETH 合约函数选择器
var (
	// submit(address referral) → 0xa1903eab
	// ETH 数量通过 tx.Value（msg.value）传入，不在 calldata 中
	SelectorLidoSubmit = [4]byte{0xa1, 0x90, 0x3e, 0xab}
)

// EncodeLidoSubmit 编码 Lido submit(address referral) calldata
// 质押的 ETH 金额由调用方设置到 Transaction.Value（msg.value）
func EncodeLidoSubmit(referral common.Address) []byte {
	data := make([]byte, 36) // 4 + 32
	copy(data[0:4], SelectorLidoSubmit[:])
	// referral address 右对齐到 32 字节
	copy(data[16:36], referral.Bytes())
	return data
}
