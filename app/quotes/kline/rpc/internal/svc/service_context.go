package svc

import (
	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/luxun9527/gex/app/quotes/kline/rpc/internal/config"
	"github.com/luxun9527/gex/app/quotes/kline/rpc/internal/dao/query"
	"github.com/luxun9527/gex/common/pkg/etcd"
	pulsarConfig "github.com/luxun9527/gex/common/pkg/pulsar"
	"github.com/luxun9527/gex/common/proto/define"
	gpushPb "github.com/luxun9527/gpush/proto"
	logger "github.com/luxun9527/zlog"
	"github.com/spf13/cast"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc/attributes"
	"strings"
)

type ServiceContext struct {
	Config        *config.Config
	Query         *query.Query
	RedisClient   *redis.Redis
	MatchConsumer pulsar.Consumer
	WsClient      gpushPb.ProxyClient
}

func NewServiceContext(c *config.Config) *ServiceContext {
	logger.InitDefaultLogger(&c.LoggerConfig)
	logx.SetWriter(logger.NewZapWriter(logger.GetZapLogger()))
	logx.DisableStat()
	//获取交易对背配置
	var symbolInfo define.SymbolInfo
	define.InitSymbolConfig(define.EtcdSymbolPrefix+c.Symbol, c.EtcdRegisterConf.EtcdConf, &symbolInfo)
	c.SymbolInfo = &symbolInfo
	logx.Infow("symbol config load ", logx.Field("symbol", symbolInfo))

	//注册到etcd
	d := strings.Split(c.RpcServerConf.ListenOn, ":")
	c.EtcdRegisterConf.Key += "/" + c.Symbol
	c.EtcdRegisterConf.Port = cast.ToInt32(d[1])
	c.EtcdRegisterConf.MetaData = attributes.New("symbol", c.Symbol)
	etcd.Register(c.EtcdRegisterConf)

	//初始化pulsar客户端
	client, err := c.PulsarConfig.BuildClient()
	if err != nil {
		logx.Severef("init pulsar client failed %v", err)
	}
	topic := pulsarConfig.Topic{
		Tenant:    pulsarConfig.PublicTenant,
		Namespace: pulsarConfig.GexNamespace,
		Topic:     pulsarConfig.MatchResultTopic + "_" + c.Symbol,
	}
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topic.BuildTopic(),
		SubscriptionName: pulsarConfig.MatchResultKlineSub,
		Type:             pulsar.Shared,
	})
	if err != nil {
		logx.Severef("init pulsar consumer failed %v", err)
	}
	sc := &ServiceContext{
		Config:        c,
		Query:         query.Use(c.GormConf.MustNewGormClient()),
		RedisClient:   redis.MustNewRedis(c.RedisConf),
		MatchConsumer: consumer,
		WsClient:      gpushPb.NewProxyClient(zrpc.MustNewClient(c.WsConf).Conn()),
	}
	return sc
}
