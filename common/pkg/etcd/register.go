package etcd

import (
	"context"
	"time"

	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/netx"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/naming/endpoints"
	"google.golang.org/grpc/attributes"
)

// 基于etcd的服务注册功能，将gRPC服务实例注册到etcd，并维持租约以确保服务的可用性，同时支持通过元数据传递额外信息，方便后续的解析和负载均衡处理。
// 核心功能特点：
// - 通过租约机制实现服务健康检查（30秒租约+自动续期）
// - 支持自动生成服务地址（IP+Port）
// - 允许携带元数据（MetaData字段），结合上下文中的balancer.go文件，可以看出这是为了支持基于交易对（如BTC_USDT）的负载均衡
// - 服务注册路径格式为conf.Key/租约ID（例如：/klineRpc/BTC_USDT/123456789）

// 与readme中方案2的关系：
// - 服务注册时携带的MetaData可以包含交易对信息
// - 结合balancer.go中的newSymbolBalancerBuilder实现自定义负载均衡
// - 通过etcd的watch机制实现服务发现，当交易对对应的服务实例变化时自动更新连接池
// - 服务注册时通过Metadata传递交易对信息，负载均衡器通过地址的Metadata构建连接池

type EtcdRegisterConf struct {
	EtcdConf EtcdConfig
	Key      string
	Value    string                 `json:",optional"`
	Port     int32                  `json:",optional"`
	MetaData *attributes.Attributes `json:",optional"`
}

func Register(conf EtcdRegisterConf) {
	go func() { // 使用goroutine异步执行注册逻辑
		// etcd客户端初始化
		cli, err := conf.EtcdConf.NewEtcdClient() // 创建etcd客户端连接
		if err != nil {
			logx.Severef("etcd new client err: %v", err)
		}
		manager, err := endpoints.NewManager(cli, conf.Key) // 创建端点管理器
		if err != nil {
			logx.Severef("etcd new manager err: %v", err)
		}
		//设置租约时间
		resp, err := cli.Grant(context.Background(), 30) // 创建30秒租约
		if err != nil {
			logx.Severef("etcd grant err: %v", err)
		}
		if conf.Value == "" {
			conf.Value = netx.InternalIp() + ":" + cast.ToString(conf.Port)
		}
		// 注册端点（使用租约ID保证唯一性）
		if err := manager.AddEndpoint(
				context.Background(), 
				conf.Key+"/"+cast.ToString(int64(resp.ID)), 
				endpoints.Endpoint{Addr: conf.Value, Metadata: conf.MetaData},  // 包含交易对信息
				clientv3.WithLease(resp.ID),
			); err != nil {
			logx.Severef("etcd add endpoint err: %v", err)
		}
		endpointMap, _ := manager.List(context.Background())
		logx.Info("List of manager:", endpointMap)
		// KeepAlive返回一个只读通道，用于接收租约续期通知
		c, err := cli.KeepAlive(context.Background(), resp.ID) // 保持租约存活
		if err != nil {
			logx.Severef("etcd keepalive err: %v", err)
		}
		logx.Infof("etcd register success,key: %v,value: %v", conf.Key, conf.Value)
		/**
		* 监听租约续期通道c，确保服务注册持续有效
		* Go语言中典型的通道(channel)监听模式，主要用于持续监控etcd租约的存活状态
		* 无限for循环中使用select语句持续监听通道c。当c有数据时，读取并忽略值，继续循环；如果c被关闭，ok变为false，记录错误并返回，退出循环，结束goroutine
		* select用于处理多个通道操作，它会阻塞直到其中一个case可以执行。这里只有一个case，但通常可能有多个case处理不同的事件
		* 关键组件解析：
			1.select语句：
			- 类似其他语言的switch，但专门用于处理通道操作
			- 同时监听多个case的通道事件
			- 当有多个case满足时随机选择一个执行
			2.通道状态处理：
			case _, ok := <-c：监听KeepAlive通道
			- _：忽略通道传递的具体值（这里只需要知道租约是否存活）
			- ok：标识通道是否正常
			- 当ok == false时表示通道已关闭（租约失效）
		* 执行流程：
			启动循环
				↓
			等待通道事件 → 通道正常 → 继续监听
				↓
			通道关闭 → 记录错误 → 退出协程
		* 结合项目上下文：
			- 这里的通道关闭意味着etcd租约失效，可能导致服务实例从注册中心移除
			- 在balancer.go中的symbolPicker会通过watch机制感知到服务实例变化，从而更新可用连接池
			- 这种设计确保了当服务实例不可用时，负载均衡器能及时剔除故障节点
			- 该goroutine的唯一职责就是维持etcd租约，不需要处理其他任务	
			- 通过纯阻塞模式可以确保100%的CPU时间都用于监控租约状态
			- 符合"do one thing and do it well"的Unix设计哲学
			- 租约失效意味着服务实例不可用，需要立即终止注册协程
		* 潜在改进点：
			- 可以添加default分支处理其他逻辑（当前代码是纯阻塞模式）
			- 可以增加超时case提升健壮性（避免永久阻塞），添加了time.After case后，这段代码不再是纯阻塞模式，而是变成了带有超时控制的阻塞模式
				case <-time.After(30 * time.Second):
					logx.Error("keepalive timeout, will retry register")
					return // 通过上层重试机制恢复服务
		* 纯阻塞模式（有意为之的设计选择）
			- 无限期等待通道事件
			- 没有其他退出途径
			- 完全被动响应
			- 纯阻塞模式比轮询模式更高效，完全依赖Go运行时调度
			- 没有default分支避免不必要的CPU空转
		* 带有超时控制的阻塞模式
			- 增加了时间维度的事件监听，与etcd租约时间(30秒)形成闭环保护
			- 30秒无响应自动超时，防止网络分区导致无限阻塞
			- 具备主动防御能力，避免goroutine泄漏
			- 超时case（30秒）可作为安全阀防止永久阻塞		  				
		*/
		for {
			select {
			case _, ok := <-c: // 监听通道事件
				if !ok { // 通道关闭时触发
					// 记录错误日志并退出协程
					logx.Errorf("etcd keepalive failed,please check etcd key %v existed", conf.Key)
					return
				}
			}
		}

	}()

}

// 新增重试包装函数备用
func RegisterWithRetry(conf EtcdRegisterConf, maxRetry int) {
    go func() {
        for i := 0; i < maxRetry; i++ {
            Register(conf) // 调用原始注册函数
            time.Sleep(time.Second * 5) // 间隔5秒重试
            logx.Infof("retry register etcd service, count: %d", i+1)
        }
    }()
}
