



# 以太坊钱包系统 (eth-scan) 开发指南

# 1. 项目目录结构 (Project Structure)

本工程采用分层架构，确保链交互逻辑与业务逻辑解耦

```htaccess
eth-scan/
├── cmd/
│   └── main.go                 # 程序入口：初始化配置、DB、Redis及扫块协程
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
    └── rpc_guide.md            # 本说明文档
```

# 2.以太坊 RPC 接口映射表



| **模块** | **接口名称 (RPC Method)**   | **业务含义**        | **核心参数 (Params)**    | **参数说明**                             |
| -------- | --------------------------- | ------------------- | ------------------------ | ---------------------------------------- |
| **基础** | `eth_getBalance`            | 查询 **ETH** 余额   | `[addr, "latest"]`       | `addr`: 20字节地址；`latest`: 最新确认块 |
| **基础** | `eth_call`                  | 查询 **ERC20** 余额 | `[{to, data}, "latest"]` | `to`: 合约地址；`data`: `balanceOf` 编码 |
| **基础** | `eth_getCode`               | 验证合约地址        | `[addr, "latest"]`       | 返回 Hex，若为 `0x` 则为普通钱包地址     |
| **扫块** | `eth_blockNumber`           | 获取当前高度        | 无                       | 用于计算扫描步长 (Current vs Latest)     |
| **扫块** | `eth_getBlockByNumber`      | 获取区块交易        | `[hex, true]`            | `true` 表示返回完整 Transaction 详情     |
| **扫块** | `eth_getTransactionReceipt` | 获取交易状态        | `[txHash]`               | 确认交易是否成功，并获取 Event Logs      |
| **扫块** | `eth_getLogs`               | 过滤充值日志        | `[{address, topics}]`    | 精准获取 Transfer 事件，无需遍历全块     |
| **业务** | `eth_getTransactionCount`   | 获取 **Nonce**      | `[addr, "pending"]`      | 连续性计数器，防止交易重放/乱序          |
| **业务** | `eth_gasPrice`              | 获取建议油价        | 无                       | 返回网络推荐的 `GasPrice` (单位: Wei)    |
| **业务** | `eth_estimateGas`           | 预估手续费          | `[{txMsg}]`              | 模拟执行，返回该操作所需的 `GasLimit`    |
| **广播** | `eth_sendRawTransaction`    | **广播交易**        | `[signedTx]`             | 发送本地私钥签名后的 Hex 序列化数据      |

# 3. 关键业务字段解析 (Transaction Data)

在以太坊中，**Data** 字段决定了交易的行为：

### **A. ERC20 代币转账 (如 USDT)**

- **函数签名 (4字节)**: `0xa9059cbb` (对应 `transfer(address,uint256)`)
- **Data 拼接规则**:
  1. `0xa9059cbb`
  2. `to_address` (左补零至 32 字节)
  3. `amount_uint256` (左补零至 32 字节)

### **B. Lido 质押 (ETH Staking)**

- **函数签名 (4字节)**: `0xa1903eab` (对应 `submit(address)`)
- **Value**: 填入要质押的 ETH 数量 (Wei)
- **Data**: `0xa1903eab` + `referral_address` (推荐人地址)

# 4.开发核心提示 (Developer Notes)

### **1. 精度陷阱 (Precision)**

以太坊原生单位是 **Wei** ($10^{18} Wei = 1 ETH$)。

- **严禁使用 `float64`** 处理金额，必须使用 `math/big` 或 `decimal` 库。
- 数据库存储建议使用 `varchar` 或 `decimal(60,0)` 存储字符串格式的大数。

### **2. Nonce 管理 (Nonce Control)**

- 同一账户在高并发提现时，`PendingNonceAt` 可能会延迟。
- **解决方案**：在内存中（如 Redis）维护一个 `Local Nonce` 计数器，并在交易失败后进行回滚或同步。

### **3. 日志截断 (Anti-Bloat)**

- 扫块时 `getBlockByNumber` 返回的数据量巨大。
- **规则**：仅解析并存储业务相关的 `hash`, `from`, `to`, `value`, `input`。禁止直接存储原始 JSON Body 到数据库或普通文本日志

### **4. 安全合规 (Security)**

- **私钥不落库**：数据库只存储加密后的 `Keystore` 或派生索引 `Index`。
- **超时处理**：所有 RPC 请求必须带 `context.WithTimeout(ctx, 10*time.Second)`。

开始