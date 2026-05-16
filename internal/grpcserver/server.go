package grpcserver

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"eth-scan/config"
	pb "eth-scan/internal/grpcserver/pb"
	"eth-scan/internal/wallet"
	"eth-scan/repository"
)

// WalletServer 实现 proto 生成的 WalletServiceServer 接口
type WalletServer struct {
	pb.UnimplementedWalletServiceServer
}

// CreateHDWallet 生成 BIP-44 HD 钱包，逻辑与 HTTP handler 完全一致
func (s *WalletServer) CreateHDWallet(_ context.Context, _ *pb.CreateHDWalletRequest) (*pb.CreateHDWalletResponse, error) {
	cfg := config.GetConfig()

	mnemonic, err := wallet.GenerateMnemonic()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "生成助记词失败: %v", err)
	}

	encrypted, err := wallet.EncryptMnemonic(mnemonic, cfg.Security.MnemonicSalt)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "加密助记词失败: %v", err)
	}

	address, err := wallet.DeriveAddress(mnemonic, 0)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "派生地址失败: %v", err)
	}

	master := &repository.MasterWallet{
		MnemonicEncrypted: encrypted,
		MnemonicPlain:     mnemonic,
		Address:           address.Hex(),
	}
	if err := repository.CreateMasterWallet(master); err != nil {
		return nil, status.Errorf(codes.Internal, "保存钱包失败: %v", err)
	}

	return &pb.CreateHDWalletResponse{
		WalletId: uint64(master.ID),
		Address:  address.Hex(),
	}, nil
}
