package etcd

import (
	"fmt"
	"time"

	"github.com/spf13/cast"
	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/balancer/base"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

//基于交易对负载均衡
//场景 撮合，订单，行情 分交易对，api 服务对这些交易对建立连接后

//key klineRpc/IKUN_USDT/xxxxx
// init函数注册了一个自定义的负载均衡器，是为了实现基于交易对的动态服务发现和负载均衡。
// 通过gRPC的balancer原生负载均衡接口实现自定义负载均衡，避免手动维护连接池
// 通过Metadata动态传递路由信息，与register.go中的服务注册形成完整服务发现体系

func init() {
	balancer.Register(newSymbolBalancerBuilder()) // 注册自定义负载均衡器
}

var (
	NotAvailableConn = status.Error(codes.Unavailable, "no available connection")
)

const SymbolLB = "symbol_lb"

// 自定义 Picker
// 按交易对分组的连接池
// 使用交易对符号(如BTC_USDT)作为key
// 每个交易对维护多个可用连接
// 通过gRPC元数据传递交易对信息
// 采用简单的时间戳取模实现轮询负载均衡
type symbolPicker struct {
	subConns map[string][]balancer.SubConn // 连接列表
}

// symbolPicker会通过watch机制感知服务实例变化
func (p *symbolPicker) Pick(info balancer.PickInfo) (balancer.PickResult, error) {
	// 如果没有可用的连接，返回错误
	if len(p.subConns) == 0 { // 依赖健康的连接池
		return balancer.PickResult{}, NotAvailableConn
	}
	// ...基于交易对的负载均衡逻辑...
	md, ok := metadata.FromIncomingContext(info.Ctx)
	if !ok {
		return balancer.PickResult{}, NotAvailableConn
	}
	symbol := md.Get("symbol")[0]   // 提取交易对符号
	conns, ok := p.subConns[symbol] // 获取该交易对的连接池
	if !ok || len(conns) == 0 {
		return balancer.PickResult{}, NotAvailableConn
	}
	index := time.Now().UnixNano() % int64(len(conns)) // 简单轮询算法
	fmt.Println("symbol:", symbol, "conns:", conns, "index:", index)
	return balancer.PickResult{SubConn: conns[index]}, nil
}

// 负载均衡器构建器
type symbolPickerBuilder struct {
	weightConfig map[string]int32
}

func (wp *symbolPickerBuilder) Build(info base.PickerBuildInfo) balancer.Picker {
	if len(info.ReadySCs) == 0 {
		return base.NewErrPicker(balancer.ErrNoSubConnAvailable)
	}
	var p = map[string][]balancer.SubConn{}
	for sc, addr := range info.ReadySCs {
		symbolData, ok := addr.Address.Metadata.(map[string]interface{})
		if !ok {
			continue
		}
		symbol := cast.ToString(symbolData["symbol"])
		if symbol == "" {
			continue
		}
		conns, ok := p[symbol]
		if !ok {
			conns = make([]balancer.SubConn, 0, 1)
		}
		conns = append(conns, sc)
		p[symbol] = conns
	}

	return &symbolPicker{
		subConns: p,
	}
}

// 自定义负载均衡
// 健康检查机制自动维护连接池状态：启用gRPC内置健康检查，自动剔除不可用连接
func newSymbolBalancerBuilder() balancer.Builder {
	return base.NewBalancerBuilder(SymbolLB, &symbolPickerBuilder{}, base.Config{HealthCheck: true})
}
