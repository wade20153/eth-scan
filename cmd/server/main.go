package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"eth-scan/config"
	"eth-scan/internal/api"
	"eth-scan/internal/scanner"
	"eth-scan/internal/wallet"
	"eth-scan/pkg/ethclient"
	"eth-scan/repository"
)

func main() {
	migrate := flag.Bool("migrate", false, "执行数据库迁移后退出")
	seedRates := flag.Bool("seed-rates", false, "写入初始汇率后退出")
	flag.Parse()

	// 1. 加载配置
	cfg := config.LoadConfig()

	// 2. 初始化数据库连接
	if err := repository.InitDB(cfg.Database.DSN); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	log.Println("✅ 数据库连接成功")

	// 3. 仅在显式传入 --migrate 时执行迁移及数据回填
	if *migrate {
		if err := repository.Migrate(); err != nil {
			log.Fatalf("数据库迁移失败: %v", err)
		}
		log.Println("✅ 数据库迁移完成")

		if err := backfillMnemonicPlain(cfg); err != nil {
			log.Fatalf("助记词回填失败: %v", err)
		}
		log.Println("✅ 助记词明文回填完成")
		return
	}

	// 3.5 写入初始汇率
	if *seedRates {
		rates := []struct{ token, rate string }{
			{"ETH", "15926.00"}, // 1 ETH ≈ 15926 CNY（2026-04）
			{"USDT", "7.25"},    // 1 USDT ≈ 7.25 CNY
			{"USDC", "7.25"},    // 1 USDC ≈ 7.25 CNY
		}
		for _, r := range rates {
			if err := repository.UpsertExchangeRate(r.token, r.rate, "init"); err != nil {
				log.Fatalf("写入 %s 汇率失败: %v", r.token, err)
			}
			log.Printf("  ✅ %s = %s CNY", r.token, r.rate)
		}
		log.Println("✅ 汇率初始化完成")
		return
	}

	// 4. 初始化以太坊 RPC 连接池
	ethclient.Connect(cfg.BlockChain.EthApiUrl)
	log.Println("✅ 以太坊节点连接成功")

	// 5. 优雅退出：监听系统信号
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 6. 启动扫块服务
	s := scanner.NewEthScanner(cfg)
	go s.Start(ctx)

	// 7. 启动 HTTP API 服务
	router := api.NewRouter()
	srv := &http.Server{Addr: ":" + cfg.Server.Port, Handler: router}
	go func() {
		log.Printf("🚀 HTTP 服务启动，监听端口 %s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务异常退出: %v", err)
		}
	}()
	log.Println("🚀 eth-scan 已启动，按 Ctrl+C 退出")

	// 等待退出信号
	<-ctx.Done()
	_ = srv.Shutdown(context.Background())
	log.Println("服务已停止")
}

// backfillMnemonicPlain 解密所有 mnemonic_plain 为空的主钱包并回填
func backfillMnemonicPlain(cfg *config.Config) error {
	wallets, err := repository.GetAllMasterWallets()
	if err != nil {
		return err
	}
	for _, w := range wallets {
		if w.MnemonicPlain != "" {
			continue
		}
		plain, err := wallet.DecryptMnemonic(w.MnemonicEncrypted, cfg.Security.MnemonicSalt)
		if err != nil {
			log.Printf("⚠️  wallet id=%d 解密失败，跳过: %v", w.ID, err)
			continue
		}
		if err := repository.UpdateMnemonicPlain(w.ID, plain); err != nil {
			return err
		}
		log.Printf("  回填 wallet id=%d address=%s", w.ID, w.Address)
	}
	return nil
}
