package grpcserver

import (
	"google.golang.org/grpc"

	pb "eth-scan/internal/grpcserver/pb"
)

// NewGRPCServer 注册所有 gRPC 服务，对应 HTTP 层的 NewRouter()
func NewGRPCServer() *grpc.Server {
	s := grpc.NewServer()

	pb.RegisterWalletServiceServer(s, &WalletServer{})
	// 后续新增服务在这里继续 Register，例如：
	// pb.RegisterBalanceServiceServer(s, &BalanceServer{})

	return s
}
