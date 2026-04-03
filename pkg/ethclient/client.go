package ethclient

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
)

// Eth 结构体封装了以太坊客户端连接池
// 通过维护多个节点连接，实现高可用性和简单的负载均衡
type Eth struct {
	clients []*ethclient.Client // 活跃的 RPC 客户端切片
	urls    []string            // 原始 RPC 地址列表
	index   uint64              // 原子计数器，用于 Round-Robin (轮询) 算法
}

var (
	// instance 全局单例，确保整个进程生命周期内只初始化一次连接池
	instance *Eth
	// once 用于保证 Connect 函数的单例执行逻辑，防止并发重复初始化
	once sync.Once
)

// GetInstance 返回初始化后的 Eth 单例
// 在调用此函数前，必须确保已经执行过 Connect
func GetInstance() *Eth {
	return instance
}

// Connect 根据传入的 URL 列表初始化连接池
// 该函数在生产环境中通常在 main 函数初始化阶段调用
func Connect(urls []string) {
	once.Do(func() {
		e := &Eth{
			urls:    urls,
			clients: make([]*ethclient.Client, 0),
		}

		for _, url := range urls {
			// 1. 设置拨号超时控制 (Dial Timeout)
			// 防止因为某个节点网络延迟高导致整个程序启动阻塞
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			// DialContext 比 Dial 更安全，支持通过 context 强制中断连接尝试
			client, err := ethclient.DialContext(ctx, url)
			if err != nil {
				// 如果单个节点连接失败，记录错误并尝试列表中的下一个节点
				// 实际生产中这里建议配合日志系统（如 zap 或 logrus）
				cancel()
				continue
			}

			// 2. 存活及可用性检查 (Health Check)
			// 连接成功不代表节点工作正常，通过获取 ChainID 来验证 JSON-RPC 响应是否正常
			_, err = client.ChainID(ctx)
			cancel() // 完成校验后立即释放 context 资源
			if err != nil {
				// 如果节点无法响应基本请求，则不加入活跃池
				continue
			}

			e.clients = append(e.clients, client)
		}

		// 3. 容错处理
		// 如果传入的所有 URL 都无法连接，程序将无法进行扫块或转账，直接 panic 强制人工介入
		if len(e.clients) == 0 {
			panic("❌ 所有 ETH 节点均不可用，请检查配置或网络。当前配置: " + string(len(urls)))
		}

		instance = e
	})
}

// GetClient 采用轮询 (Round-Robin) 算法获取一个活跃的客户端
// 这种方式可以平摊单体 RPC (如 Infura/Alchemy) 的 CU (计算单元) 消耗压力
func (e *Eth) GetClient() *ethclient.Client {
	if len(e.clients) == 0 {
		return nil
	}
	// 使用原子操作 (Atomic) 增加索引值，确保多线程并发获取 Client 时：
	// 1. 索引递增是线程安全的
	// 2. 负载能够均匀分布在各个节点上
	idx := atomic.AddUint64(&e.index, 1)
	// 通过取模运算实现循环轮询
	return e.clients[idx%uint64(len(e.clients))]
}
