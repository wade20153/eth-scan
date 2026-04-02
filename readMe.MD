corp_saas_eth_crawler/
├── cmd/
│   └── main.go                 # 程序入口，初始化配置、数据库和协程
├── config/
│   └── config.go               # 读取 viper 配置（RPC 节点、合约地址、私钥加密盐）
├── internal/
│   ├── scanner/
│   │   └── eth_scanner.go      # 核心扫块逻辑：轮询高度、解析 Block、识别充值
│   ├── wallet/
│   │   ├── account.go          # 账户生成、助记词派生路径管理
│   │   ├── transaction.go      # 构建交易、Gas 预估、Nonce 管理
│   │   └── balance.go          # 余额查询（支持 ETH 和多合约 ERC20）
│   ├── staking/
│   │   └── lido_handler.go     # 质押逻辑：封装与 Lido/RocketPool 合约的交互
│   └── contract/
│       ├── erc20_abi.go        # 使用 abigen 生成的 ERC20 Go 绑定文件
│       └── lido_abi.go         # 质押合约的 Go 绑定文件
├── pkg/
│   ├── ethclient/
│   │   └── client.go           # 封装 go-ethereum 客户端单例与重试机制
│   └── utils/
│       └── unit_convert.go     # Wei 与 Ether 之间的精度转换 (big.Int)
├── repository/
│   ├── model.go                # GORM 数据库定义 (Accounts, Txs, Logs)
│   └── dao.go                  # 数据库增删改查
└── docs/
└── swagger.json            # 由 swag init 生成的接口文档