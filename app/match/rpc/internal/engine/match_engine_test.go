package engine_test

import (
	"testing"
	"time"

	pulsar_client "github.com/apache/pulsar-client-go/pulsar"
	"github.com/luxun9527/gex/app/match/rpc/internal/config"
	"github.com/luxun9527/gex/app/match/rpc/internal/engine"
	pulsarConfig "github.com/luxun9527/gex/common/pkg/pulsar"
	"github.com/luxun9527/gex/common/proto/define"
	"github.com/luxun9527/gex/common/proto/enum"
	"github.com/luxun9527/gex/common/utils"
	gpush "github.com/luxun9527/gpush/proto"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/yitter/idgenerator-go/idgen"
	"github.com/zeromicro/go-zero/core/discov"
	"github.com/zeromicro/go-zero/zrpc"
)

// 本测试文件包含以下主要测试用例:
// 1.TestMatchLimitBuyOrder - 测试限价买单撮合
// 2.TestMatchLimitSellOrder - 测试限价卖单撮合  
// 3.TestMatchMarketBuyOrder - 测试市价买单撮合
// 4.TestMatchMarketSellOrder - 测试市价卖单撮合

// 每个测试用例都会:
// 1.创建测试用的 MatchEngine 实例
// 2.添加测试订单
// 3.验证撮合结果

// 确保测试环境中 Pulsar 和 Etcd 服务可访问,否则测试会失败。
// 测试过程中会创建撮合引擎实例、订单簿,并验证撮合结果的正确性。
// cd /git/gex/app/match/rpc/internal/engine
// go test -v match_engine_test.go

// 使用 -run 参数运行特定测试用例
// go test -v -run TestMatchLimitBuyOrder match_engine_test.go

// go test -v app/match/rpc/internal/engine/match_engine_test.go

// 创建测试用的MatchEngine实例
func createTestMatchEngine() *engine.MatchEngine {
	// 创建配置
	c := &config.Config{
		Symbol: "BTC_USDT",
		SymbolInfo: &define.SymbolInfo{
			SymbolID:           1,
			SymbolName:        "BTC_USDT",
			BaseCoinID:        1,
			QuoteCoinID:       2,
			BaseCoinPrecValue: 4,
			QuoteCoinPrecValue: 4,
			BaseCoinName:      "BTC",
			QuoteCoinName:     "USDT",
		},
		PulsarConfig: pulsarConfig.PulsarConfig{
			Hosts: []string{"pulsar:6650"},
		},
		WsConf:  zrpc.RpcClientConf{
			Etcd: discov.EtcdConf{
				Hosts: []string{"etcd:2379"},
				Key: "proxy",
			},
		},
	}

	// 创建Pulsar客户端
	client, err := c.PulsarConfig.BuildClient()
	if err != nil {
		panic(err)
	}
	// 创建生产者
	// 撮合引擎通过producer向Pulsar发送撮合结果
	topic := pulsarConfig.Topic{
		Tenant:    pulsarConfig.PublicTenant,  // public
		Namespace: pulsarConfig.GexNamespace,  // trade
		Topic:     pulsarConfig.MatchResultTopic + "_" + c.Symbol, // match_result_BTC_USDT
	}
	producer, err := client.CreateProducer(pulsar_client.ProducerOptions{
		// Topic: "test_match_result",
		Topic:           topic.BuildTopic(),
		SendTimeout:     10 * time.Second,
		DisableBatching: true, // 禁用批处理
	})
	if err != nil {
		panic(err)
	}
	// 创建WebSocket代理客户端
	proxyClient := gpush.NewProxyClient(zrpc.MustNewClient(c.WsConf).Conn())
    if err != nil {
        panic(err)
    }
	return engine.NewMatchEngine(c, producer, proxyClient)
}
// 创建限价单
func createLimitOrder(id int64, price string, qty string, side enum.Side) *engine.Order {
	return &engine.Order{
		OrderID:        "test_order",
		SequenceId:     id,
		Side:          side,
		OrderType:     enum.OrderType_LO,
		Price:         utils.NewFromStringMaxPrec(price),
		Qty:           utils.NewFromStringMaxPrec(qty),
		UnfilledQty:   utils.NewFromStringMaxPrec(qty),
		Amount:        utils.NewFromStringMaxPrec(price).Mul(utils.NewFromStringMaxPrec(qty)),
		UnfilledAmount: utils.NewFromStringMaxPrec(price).Mul(utils.NewFromStringMaxPrec(qty)),
	}
}
// 创建市价单
func createMarketOrder(id int64, amount string, qty string, side enum.Side) *engine.Order {
	return &engine.Order{
		OrderID:    "test_order",
		SequenceId: id,
		Side:       side,
		OrderType:  enum.OrderType_MO,
		Price:      decimal.Zero,
		Qty:        utils.NewFromStringMaxPrec(qty),
		UnfilledQty: utils.NewFromStringMaxPrec(qty),
		Amount:     utils.NewFromStringMaxPrec(amount),
		UnfilledAmount: utils.NewFromStringMaxPrec(amount),
	}
}
// 测试限价买单撮合
func TestMatchLimitBuyOrder(t *testing.T) {
	// 初始化idgen
	idgen.SetIdGenerator(&idgen.IdGeneratorOptions{
		WorkerId: 1, 
		BaseTime: time.Now().UnixMilli(), 
		WorkerIdBitLength: 6, 
		SeqBitLength: 6, 
		MaxSeqNumber: 0, 
		MinSeqNumber: 5, 
		TopOverCostCount: 2000})
	me := createTestMatchEngine()
	
	// 添加卖单
	sellOrder := createLimitOrder(1, "100", "1", enum.Side_Sell)
	me.HandleOrder(sellOrder)
	
	// 添加买单
	buyOrder := createLimitOrder(2, "100", "0.5", enum.Side_Buy)
	me.HandleOrder(buyOrder)
	
	// 验证撮合结果
	// 1.买单数量为0.5,卖单数量为1
	// 2.撮合后买单全部成交(FilledQty=0.5)
	// 3.卖单部分成交(FilledQty=0.5)
	assert.Equal(t, "0.5", buyOrder.FilledQty.String())
	assert.Equal(t, "50", buyOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_ALLFilled, buyOrder.OrderStatus)
	
	assert.Equal(t, "0.5", sellOrder.FilledQty.String())
	assert.Equal(t, "50", sellOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_PartFilled, sellOrder.OrderStatus)
}
// 测试限价卖单撮合
func TestMatchLimitSellOrder(t *testing.T) {
	// 初始化idgen
	idgen.SetIdGenerator(&idgen.IdGeneratorOptions{
		WorkerId: 1, 
		BaseTime: time.Now().UnixMilli(), 
		WorkerIdBitLength: 6, 
		SeqBitLength: 6, 
		MaxSeqNumber: 0, 
		MinSeqNumber: 5, 
		TopOverCostCount: 2000})	
	me := createTestMatchEngine()
	
	// 添加买单
	buyOrder := createLimitOrder(1, "100", "1", enum.Side_Buy)
	me.HandleOrder(buyOrder)
	
	// 添加卖单
	sellOrder := createLimitOrder(2, "100", "0.5", enum.Side_Sell)
	me.HandleOrder(sellOrder)
	
	// 验证撮合结果
	assert.Equal(t, "0.5", sellOrder.FilledQty.String())
	assert.Equal(t, "50", sellOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_ALLFilled, sellOrder.OrderStatus)
	
	assert.Equal(t, "0.5", buyOrder.FilledQty.String())
	assert.Equal(t, "50", buyOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_PartFilled, buyOrder.OrderStatus)
}
// 测试市价买单撮合
func TestMatchMarketBuyOrder(t *testing.T) {
	// 初始化idgen
	idgen.SetIdGenerator(&idgen.IdGeneratorOptions{
		WorkerId: 1, 
		BaseTime: time.Now().UnixMilli(), 
		WorkerIdBitLength: 6, 
		SeqBitLength: 6, 
		MaxSeqNumber: 0, 
		MinSeqNumber: 5, 
		TopOverCostCount: 2000})	
	me := createTestMatchEngine()
	
	// 添加卖单
	sellOrder := createLimitOrder(1, "100", "1", enum.Side_Sell)
	me.HandleOrder(sellOrder)
	
	// 添加市价买单,金额100
	buyOrder := createMarketOrder(2, "100", "1", enum.Side_Buy)
	me.HandleOrder(buyOrder)
	
	// 验证撮合结果
	assert.Equal(t, "1", buyOrder.FilledQty.String())
	assert.Equal(t, "100", buyOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_ALLFilled, buyOrder.OrderStatus)
	
	assert.Equal(t, "1", sellOrder.FilledQty.String())
	assert.Equal(t, "100", sellOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_ALLFilled, sellOrder.OrderStatus)
}
// 测试市价卖单撮合
func TestMatchMarketSellOrder(t *testing.T) {
	// 初始化idgen
	idgen.SetIdGenerator(&idgen.IdGeneratorOptions{
		WorkerId: 1, 
		BaseTime: time.Now().UnixMilli(), 
		WorkerIdBitLength: 6, 
		SeqBitLength: 6, 
		MaxSeqNumber: 0, 
		MinSeqNumber: 5, 
		TopOverCostCount: 2000})	
	me := createTestMatchEngine()
	
	// 添加买单
	buyOrder := createLimitOrder(1, "100", "1", enum.Side_Buy)
	me.HandleOrder(buyOrder)
	
	// 添加市价卖单,数量0.5
	sellOrder := createMarketOrder(2, "50", "0.5", enum.Side_Sell)
	me.HandleOrder(sellOrder)
	
	// 验证撮合结果
	assert.Equal(t, "0.5", sellOrder.FilledQty.String())
	assert.Equal(t, "50", sellOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_ALLFilled, sellOrder.OrderStatus)
	
	assert.Equal(t, "0.5", buyOrder.FilledQty.String())
	assert.Equal(t, "50", buyOrder.FilledAmount.String())
	assert.Equal(t, enum.OrderStatus_PartFilled, buyOrder.OrderStatus)
}