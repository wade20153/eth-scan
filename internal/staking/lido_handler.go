package staking

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"eth-scan/internal/contract"
	"eth-scan/pkg/ethclient"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

// LidoStakeAddress Lido stETH 合约地址（主网）
const LidoStakeAddress = "0xae7ab96520DE3A18E5e111B5EaAb095312D7fE84"

// Stake 向 Lido 质押 ETH，获得等值 stETH
// fromPrivKeyHex: 发送方私钥（十六进制，不含 0x 前缀）
// amountWei: 质押金额（单位 Wei）
// referral: 推荐人地址（无推荐人填 common.Address{}）
// chainID: 网络 ID（主网=1，Sepolia=11155111）
func Stake(fromPrivKeyHex string, amountWei *big.Int, referral common.Address, chainID *big.Int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	privateKey, err := crypto.HexToECDSA(fromPrivKeyHex)
	if err != nil {
		return "", fmt.Errorf("私钥解析失败: %w", err)
	}
	from := crypto.PubkeyToAddress(privateKey.PublicKey)

	lidoAddr := common.HexToAddress(LidoStakeAddress)
	callData := contract.EncodeLidoSubmit(referral)

	// 预估 Gas
	gasLimit, err := ethclient.GetInstance().EstimateGas(ctx, ethereum.CallMsg{
		From:  from,
		To:    &lidoAddr,
		Value: amountWei,
		Data:  callData,
	})
	if err != nil {
		return "", fmt.Errorf("预估 Gas 失败: %w", err)
	}

	gasPrice, err := ethclient.GetInstance().GetSuggestGasPrice(ctx)
	if err != nil {
		return "", fmt.Errorf("获取 GasPrice 失败: %w", err)
	}

	nonce, err := ethclient.GetInstance().GetTransactionCount(ctx, from)
	if err != nil {
		return "", fmt.Errorf("获取 Nonce 失败: %w", err)
	}

	// 质押金额放入 tx.Value（msg.value）
	tx := types.NewTransaction(nonce, lidoAddr, amountWei, gasLimit, gasPrice, callData)
	signer := types.NewEIP155Signer(chainID)
	signedTx, err := types.SignTx(tx, signer, privateKey)
	if err != nil {
		return "", fmt.Errorf("签名失败: %w", err)
	}

	if err = ethclient.GetInstance().SendRawTransaction(ctx, signedTx); err != nil {
		return "", fmt.Errorf("广播失败: %w", err)
	}
	return signedTx.Hash().Hex(), nil
}
