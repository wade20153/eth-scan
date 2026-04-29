package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/tyler-smith/go-bip32"
	"github.com/tyler-smith/go-bip39"
)

const (
	bip44Purpose = bip32.FirstHardenedChild + 44
	bip44CoinETH = bip32.FirstHardenedChild + 60
	bip44Account = bip32.FirstHardenedChild + 0
	bip44Change  = uint32(0)
)

// GenerateMnemonic 生成 12 个单词的 BIP-39 助记词
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128) // 128 bit → 12 words那么
	if err != nil {
		return "", fmt.Errorf("生成熵失败: %w", err)
	}
	return bip39.NewMnemonic(entropy)
}

// DeriveAddress 从助记词按 BIP-44 路径派生子地址
// 路径：m/44'/60'/0'/0/{index}
func DeriveAddress(mnemonic string, index uint32) (address common.Address, err error) {
	_, address, err = deriveKey(mnemonic, index)
	return
}

// DerivePrivateKeyHex 派生私钥十六进制字符串（仅在签名时调用，用后立即释放）
func DerivePrivateKeyHex(mnemonic string, index uint32) (string, common.Address, error) {
	privateKey, address, err := deriveKey(mnemonic, index)
	if err != nil {
		return "", common.Address{}, err
	}
	keyHex := hex.EncodeToString(crypto.FromECDSA(privateKey))
	return keyHex, address, nil
}

// EncryptMnemonic 使用 AES-256-GCM 加密助记词
func EncryptMnemonic(mnemonic, salt string) (string, error) {
	key := sha256.Sum256([]byte(salt))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(mnemonic), nil)
	return hex.EncodeToString(ciphertext), nil
}

// DecryptMnemonic 解密助记词
func DecryptMnemonic(encrypted, salt string) (string, error) {
	key := sha256.Sum256([]byte(salt))
	data, err := hex.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("密文长度不足")
	}
	plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
	return string(plaintext), err
}

// deriveKey 内部派生私钥，路径 m/44'/60'/0'/0/{index}
func deriveKey(mnemonic string, index uint32) (*ecdsa.PrivateKey, common.Address, error) {
	// 1. 助记词 → 种子
	seed := bip39.NewSeed(mnemonic, "")

	// 2. 种子 → BIP-32 主密钥
	masterKey, err := bip32.NewMasterKey(seed)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("生成主密钥失败: %w", err)
	}

	// 3. 按 BIP-44 路径逐级派生
	path := []uint32{bip44Purpose, bip44CoinETH, bip44Account, bip44Change, index}
	key := masterKey
	for _, child := range path {
		key, err = key.NewChildKey(child)
		if err != nil {
			return nil, common.Address{}, fmt.Errorf("派生子密钥失败 (index=%d): %w", child, err)
		}
	}

	// 4. BIP-32 私钥字节 → Ethereum ECDSA 私钥
	privateKey, err := crypto.ToECDSA(key.Key)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("转换私钥失败: %w", err)
	}

	// 5. 公钥 → 以太坊地址
	address := crypto.PubkeyToAddress(privateKey.PublicKey)
	return privateKey, address, nil
}
