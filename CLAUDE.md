# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 常用命令

```bash
# 构建
go build -o eth-scan ./cmd/

# 运行
go run ./cmd/

# 测试
go test ./...
go test ./pkg/ethclient/...   # 单独测试某个包

# 代码检查（需安装 golangci-lint）
golangci-lint run
```

## 项目目录结构

本工程采用分层架构，确保链交互逻辑与业务逻辑解耦。**以下目录为权威规范，新功能须按此结构开发**：

```
eth-scan/
├── cmd/
│   └── main.go                 # 程序入口：初始化配置、DB、Redis 及扫块协程
├── config/
│   └── config.go               # 配置加载：RPC URL、合约地址、加密盐(Salt)
├── internal/
│   ├── scanner/
│   │   └── eth_scanner.go      # 核心扫块：轮询区块、解析 Transaction/Logs、识别充值
│   ├── wallet/
│   │   ├── account.go          # 账户系统：BIP-44 助记词派生、私钥加密存储
│   │   ├── transaction.go      # 交易系统：离线签名、Nonce 管理、Gas 预估、广播
│   │   └── balance.go          # 资产查询：ETH 原生余额及多 ERC20 合约余额
│   ├── staking/
│   │   └── lido_handler.go     # 质押业务：封装 Lido/RocketPool 智能合约交互
│   └── contract/
│       ├── erc20_abi.go        # abigen 生成的 ERC20 交互接口
│       └── lido_abi.go         # abigen 生成的质押合约交互接口
├── pkg/
│   ├── ethclient/
│   │   └── client.go           # RPC 客户端封装：单例模式、重试机制、超时控制
│   └── utils/
│       └── unit_convert.go     # 精度转换：Wei 与 Ether (10^18) 安全转换
├── repository/
│   ├── model.go                # GORM 模型：Accounts, Transactions, ScanHeight
│   └── dao.go                  # 数据访问：CRUD 操作
└── docs/
    └── rpc_interfaces.md       # RPC 接口规范及开发指南（本项目权威文档）
```

## 核心组件说明

**`pkg/ethclient/client.go`** — 连接池单例，连接多个 RPC 节点，原子计数器轮询负载均衡，启动时用 `ChainID` 验证连通性。

**`internal/scanner/eth_scanner.go`** — 扫块核心循环：加载监控地址 → 获取最新块高 → 落后 12 块扫描（确认数）→ 匹配 `tx.To` → 调用 `handleMatchedTx`。`sync.RWMutex` 支持地址表动态更新，`context.Done()` 优雅退出。

**`internal/wallet/`** — 钱包三件套：`account.go` 负责 BIP-44 派生与 Keystore 加密；`transaction.go` 负责离线签名、Nonce 管理、Gas 预估与广播；`balance.go` 负责 ETH 及 ERC20 余额查询。

**`pkg/utils/unit_convert.go`** — Wei ↔ Ether 精度安全转换，禁止在此之外用 `float64` 处理金额。

## 以太坊 RPC 接口映射

| 模块 | 接口 | 业务含义 |
|------|------|---------|
| 基础 | `eth_getBalance` | 查询 ETH 余额 |
| 基础 | `eth_call` | 查询 ERC20 余额（`balanceOf` 编码） |
| 基础 | `eth_getCode` | 验证是否为合约地址（返回 `0x` 则为普通钱包） |
| 扫块 | `eth_blockNumber` | 获取当前块高 |
| 扫块 | `eth_getBlockByNumber` | 获取区块完整交易（参数 `true`） |
| 扫块 | `eth_getTransactionReceipt` | 获取交易状态及 Event Logs |
| 扫块 | `eth_getLogs` | 过滤 Transfer 事件，无需遍历全块 |
| 业务 | `eth_getTransactionCount` | 获取 Nonce（`"pending"`） |
| 业务 | `eth_gasPrice` | 获取建议 GasPrice（Wei） |
| 业务 | `eth_estimateGas` | 预估 GasLimit |
| 广播 | `eth_sendRawTransaction` | 广播已签名交易 |

## 关键实现规范

**精度**：禁止用 `float64` 处理金额，必须用 `math/big.Int`（1 ETH = 10^18 Wei）。统一通过 `pkg/utils/unit_convert.go` 转换。数据库用 `varchar` 或 `decimal(60,0)`。

**Nonce 管理**：高并发下 `PendingNonceAt` 存在延迟，需在 Redis/内存中维护本地 Nonce 计数器，失败时回滚同步。

**日志截断**：`eth_getBlockByNumber` 数据量巨大，只解析并存储 `hash`、`from`、`to`、`value`、`input`，禁止将原始 JSON 存入数据库或日志。

**超时控制**：所有 RPC 调用必须使用 `context.WithTimeout(ctx, 10*time.Second)`。

**私钥安全**：数据库只存加密后的 Keystore 或派生索引 `Index`，禁止明文私钥落库。

## calldata 编码参考

ERC20 转账（`transfer(address,uint256)`）：`0xa9059cbb` + 接收地址（左补零至 32 字节）+ 金额（左补零至 32 字节）

Lido 质押（`submit(address)`）：`0xa1903eab` + referral 地址（32 字节），质押 ETH 数量通过 `msg.value`（Wei）传入
