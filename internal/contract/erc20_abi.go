package contract

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

// ERC20 标准函数选择器（Keccak256 前 4 字节）
var (
	// transfer(address,uint256) → 0xa9059cbb
	SelectorTransfer = [4]byte{0xa9, 0x05, 0x9c, 0xbb}
	// balanceOf(address) → 0x70a08231
	SelectorBalanceOf = [4]byte{0x70, 0xa0, 0x82, 0x31}
	// Transfer(address,address,uint256) 事件签名（用于 eth_getLogs 过滤）
	// keccak256("Transfer(address,address,uint256)")
	TopicTransfer = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
)

// EncodeBalanceOf 编码 balanceOf(address) calldata
func EncodeBalanceOf(owner common.Address) []byte {
	data := make([]byte, 36) // 4 + 32
	copy(data[0:4], SelectorBalanceOf[:])
	// address 右对齐到 32 字节（前 12 字节补零）
	copy(data[16:36], owner.Bytes())
	return data
}

// EncodeTransfer 编码 transfer(address,uint256) calldata
func EncodeTransfer(to common.Address, amount *big.Int) []byte {
	data := make([]byte, 68) // 4 + 32 + 32
	copy(data[0:4], SelectorTransfer[:])
	// param1: to address，右对齐到 32 字节
	copy(data[16:36], to.Bytes())
	// param2: amount，右对齐到 32 字节
	amtBytes := amount.Bytes()
	copy(data[36+(32-len(amtBytes)):68], amtBytes)
	return data
}

// DecodeUint256 将 ABI 编码的 32 字节解码为 big.Int
func DecodeUint256(data []byte) *big.Int {
	if len(data) < 32 {
		return big.NewInt(0)
	}
	return new(big.Int).SetBytes(data[:32])
}
