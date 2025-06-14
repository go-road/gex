package engine

import (
	"context"
	"fmt"
	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/luxun9527/gex/app/match/rpc/internal/config"
	enum "github.com/luxun9527/gex/common/proto/enum"
	matchMq "github.com/luxun9527/gex/common/proto/mq/match"
	commonWs "github.com/luxun9527/gex/common/proto/ws"
	"github.com/luxun9527/gex/common/utils"
	ws "github.com/luxun9527/gpush/proto"
	logger "github.com/luxun9527/zlog"
	"github.com/shopspring/decimal"
	"github.com/spf13/cast"
	"github.com/yitter/idgenerator-go/idgen"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stringx"
	"google.golang.org/protobuf/proto"
	"log"
	"math"
	"time"
)

/**
撮合流程:

1.限价单撮合:
	-买单从卖盘中找对手方,卖单从买盘中找对手方
	-判断价格是否匹配(买单价格>=卖单价格)
	-按照时间优先、价格优先原则撮合
	-部分成交的订单继续留在订单簿中
2.市价单撮合:
	-市价买单按金额撮合,从卖一价开始往上吃单
	-市价卖单按数量撮合,从买一价开始往下吃单
	-直到完全成交或无对手方

3.撮合结果处理:
	-更新订单状态
	-发送撮合结果消息
	-更新深度数据
	-推送行情数据
整个撮合引擎基于内存撮合,使用红黑树存储订单簿,保证了订单按价格排序的高效性。撮合结果通过消息队列异步处理,保证了撮合的高性能。

这是一个典型的交易所撮合引擎实现,包含了订单簿管理、价格撮合、深度维护等核心功能。代码结构清晰,性能优化合理。	

增加了更新订单已成交数量的逻辑：
每次撮合成功后,都需要同时更新 taker 和 maker 订单的已成交数量(FilledQty)。更新采用累加方式,使用 Add() 方法将本次成交数量(qty)加到原有的已成交数量上。
无论是限价单还是市价单,买单卖单都遵循相同的更新逻辑。更新操作在每次撮合匹配成功后立即执行。
限价买单(matchLimitOrderBuy)
限价卖单(matchLimitOrderSell)
市价买单(matchMarkerOrderBuy)
市价卖单(matchMarketOrderSell)
确保了:
1.订单成交数量的准确记录
2.Taker和Maker订单状态的同步更新 
3.成交历史的完整追踪
4.订单状态的实时反映
*/

// MatchEngine 撮合引擎
type MatchEngine struct {
	asks             *OrderBook      //卖盘
	bids             *OrderBook      //买盘
	bestBid          decimal.Decimal //买一价
	bestAsk          decimal.Decimal //卖一价
	baseCoinMinUnit  decimal.Decimal
	quoteCoinMinUnit decimal.Decimal
	depthHandler     *DepthHandler
	c                *config.Config
	producer         pulsar.Producer
	proxyClient      ws.ProxyClient
	tick             chan *MatchResult
	currentSeqId     int64
}

// MatchedRecord  一次撮合匹配的结果,一次撮合会多次匹配
type MatchedRecord struct {
	Price           decimal.Decimal
	Qty             decimal.Decimal
	Amount          decimal.Decimal
	MatchedRecordID string
	//最新的taker订单的状态
	Taker Order
	//最新的maker订单的状态
	Maker Order
}
type MatchResult struct {
	//每一次匹配的结果 撮合记录
	MatchedRecords []*MatchedRecord
	//本次撮合的id
	MatchID    string
	CancelResp *CancelResp
	//撮合时间
	MatchTime int64
	//taker是否是买单
	TakerIsBuy bool
}
type CancelResp struct {
	//取消订单的id，如果不为空则表示取消订单。
	CancelId int64
	//币种id 取消买单则为计价币id，取消卖单则为基础币id
	CoinId int32
	//数量 取消买单则为计价币数量，取消卖单则为基础币数量
	Qty string
	//用户id
	Uid int64
}

func (mr *MatchResult) println() {
	fmt.Printf("============================================================\n")
	fmt.Printf("matchID=%v cancelOrderID=%v\n", mr.MatchID, mr.CancelResp)
	for _, v := range mr.MatchedRecords {
		fmt.Printf("matchID=%v matchedRecord Price=%v qty=%v Qty=%v\n ", v.MatchedRecordID, v.Price, v.Qty, v.Amount)
		fmt.Printf("taker=%+v\n", v.Taker)
		fmt.Printf("maker=%+v\n", v.Maker)
	}
	fmt.Printf("============================================================\n")

}

func NewMatchEngine(c *config.Config, producer pulsar.Producer, proxyClient ws.ProxyClient) *MatchEngine {
	me := &MatchEngine{
		asks:         NewOrderBook(enum.Side_Sell),
		bids:         NewOrderBook(enum.Side_Buy),
		bestBid:      utils.DecimalZeroMaxPrec,
		bestAsk:      utils.DecimalZeroMaxPrec,
		depthHandler: NewDepthHandler(0, c, proxyClient),
		c:            c,
		producer:     producer,
		proxyClient:  proxyClient,
		tick:         make(chan *MatchResult, 10),
	}
	go me.sendTick()
	return me
}

func (m *MatchEngine) addOrder(order *Order) {
	if order.Side == enum.Side_Buy {
		m.bids.add(order)
		m.updateBestBid()
	} else {
		m.asks.add(order)
		m.updateBestAsk()
	}

}
func (m *MatchEngine) cancelOrder(order *Order) {
	if order.Side == enum.Side_Buy {
		m.bids.remove(order)
		m.updateBestBid()
	} else {
		m.asks.remove(order)
		m.updateBestAsk()
	}
}

// 更新买一价
func (m *MatchEngine) updateBestBid() {
	if m.bids.orderBook.Size() == 0 {
		m.bestBid = utils.DecimalZeroMaxPrec
	} else {
		m.bestBid = m.bids.orderBook.Left().Key.(*Key).price
	}
}

// 更新卖一价
func (m *MatchEngine) updateBestAsk() {
	if m.asks.orderBook.Size() == 0 {
		m.bestAsk = utils.DecimalZeroMaxPrec
	} else {
		m.bestAsk = m.asks.orderBook.Left().Key.(*Key).price
	} 
}

// 匹配市价单卖单
func (m *MatchEngine) matchMarketOrderSell(takerOrder *Order) {

	matchedResult := &MatchResult{
		MatchedRecords: make([]*MatchedRecord, 0, 2),
		TakerIsBuy:     false,
	}
	//如果没有买盘，直接取消订单
	if m.bids.orderBook.Size() == 0 {
		matchedResult.CancelResp = &CancelResp{
			CancelId: takerOrder.SequenceId,
			CoinId:   m.c.SymbolInfo.BaseCoinID,
			Qty:      takerOrder.UnfilledQty.String(),
			Uid:      takerOrder.Uid,
		}
		m.SendMatchResult(matchedResult)
		return
	}

	iterator := m.bids.orderBook.Iterator()
	var matchedRecord *MatchedRecord
	deletedKeys := make([]*Key, 0, 2)
	for iterator.Next() {
		makerOrder := iterator.Value().(*Order)
		result := takerOrder.UnfilledQty.Cmp(makerOrder.UnfilledQty)
		switch {
		case result == 1:
			takerOrder.OrderStatus = enum.OrderStatus_PartFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			takerOrder.UnfilledQty = takerOrder.UnfilledQty.Sub(qty)
			//takerOrder.UnfilledAmount = takerOrder.UnfilledAmount.Sub(amount)
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{
				//订单的剩余数量
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))
		case result == 0:
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled

			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			takerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量

			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{

				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))
		case result == -1:
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			makerOrder.OrderStatus = enum.OrderStatus_PartFilled

			//更新订单的剩余数量
			qty := takerOrder.UnfilledQty
			//	amount := takerOrder.UnfilledAmount
			a := qty.Mul(makerOrder.Price)
			takerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = makerOrder.UnfilledQty.Sub(qty)
			makerOrder.UnfilledAmount = makerOrder.UnfilledAmount.Sub(a)
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(a)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(a)
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量

			//订单的剩余数量
			matchedRecord = &MatchedRecord{

				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: a,
			}
		}
		matchedRecord.Taker = *takerOrder
		matchedRecord.Maker = *makerOrder
		matchedRecord.MatchedRecordID = cast.ToString(idgen.NextId())
		matchedResult.MatchedRecords = append(matchedResult.MatchedRecords, matchedRecord)
		//订单全部成交退出，或者小于下一个订单的价格。不再循环匹配。
		if takerOrder.OrderStatus == enum.OrderStatus_ALLFilled {
			break
		}

	}
	//删除买盘被匹配过的订单，更新买一价
	if len(deletedKeys) > 0 {
		for _, v := range deletedKeys {
			m.bids.orderBook.Remove(v)
		}
		m.updateBestBid()
	}
	//更新深度数据
	for _, record := range matchedResult.MatchedRecords {
		p := &position{
			price: record.Price,
			qty:   record.Qty,
		}
		m.depthHandler.updateDepth(p, enum.Side_Buy, Delete, m.currentSeqId)
	}
	matchedResult.MatchTime = time.Now().UnixNano()
	matchedResult.MatchID = cast.ToString(idgen.NextId())
	m.SendMatchResult(matchedResult)

	if takerOrder.OrderStatus != enum.OrderStatus_ALLFilled {
		r := &MatchResult{
			CancelResp: &CancelResp{
				CancelId: takerOrder.SequenceId,
				CoinId:   m.c.SymbolInfo.BaseCoinID,
				Qty:      takerOrder.UnfilledQty.String(),
				Uid:      takerOrder.Uid,
			},
		}
		//发送取消订单
		m.SendMatchResult(r)

	}
}

// 市价买单 按照指定计价币的数量来买
func (m *MatchEngine) matchMarkerOrderBuy(takerOrder *Order) {

	matchedResult := &MatchResult{
		MatchedRecords: make([]*MatchedRecord, 0, 2),
		TakerIsBuy:     true,
	}
	//如果没有卖盘，直接取消订单
	if m.asks.orderBook.Size() == 0 {
		matchedResult.CancelResp = &CancelResp{
			CancelId: takerOrder.SequenceId,
			CoinId:   m.c.SymbolInfo.QuoteCoinID,
			Qty:      takerOrder.UnfilledAmount.String(),
			Uid:      takerOrder.Uid,
		}
		//m.Next <- matchedResult
		m.SendMatchResult(matchedResult)
		return
	}

	iterator := m.asks.orderBook.Iterator()
	//待被删除的key
	deletedKeys := make([]*Key, 0, 2)
	var matchedRecord *MatchedRecord
LOOP:
	for iterator.Next() {
		makerOrder := iterator.Value().(*Order)
		result := takerOrder.UnfilledAmount.Cmp(makerOrder.UnfilledAmount)
		switch result {
		case 1:
			takerOrder.OrderStatus = enum.OrderStatus_PartFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量
			takerOrder.UnfilledAmount = takerOrder.UnfilledAmount.Sub(amount)
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec

			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{
				//订单的剩余数量
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))
		case 0:
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量	
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
		
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))

		case -1:
			//taker金额比maker的金额要小，匹配结束
			//按照taker的金额购买的话能买多少，小于最小的单位则结束。
			qty := takerOrder.UnfilledAmount.Div(makerOrder.Price)
			baseCoinMinUnit := utils.NewFromStringMaxPrec(cast.ToString(math.Pow10(int(-m.c.SymbolInfo.BaseCoinPrec.Load()))))
			if qty.LessThan(baseCoinMinUnit) {
				break LOOP
			}
			makerOrder.OrderStatus = enum.OrderStatus_PartFilled
			takerOrder.OrderStatus = enum.OrderStatus_PartFilled
			//去除余数
			//数量
			q := qty.Div(baseCoinMinUnit).Floor().Mul(baseCoinMinUnit)
			//金额
			a := q.Mul(makerOrder.Price)
			//更新订单的剩余数量
			takerOrder.FilledQty = takerOrder.FilledQty.Add(q)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(q)  // 更新已成交数量
			takerOrder.UnfilledAmount = takerOrder.UnfilledAmount.Sub(a)
			makerOrder.UnfilledQty = makerOrder.UnfilledQty.Sub(q)
			makerOrder.UnfilledAmount = makerOrder.UnfilledAmount.Sub(a)
			if takerOrder.UnfilledAmount.Equal(utils.DecimalZeroMaxPrec) {
				takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			}
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(a)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(a)
			matchedRecord = &MatchedRecord{

				Price:  makerOrder.Price,
				Qty:    q,
				Amount: a,
			}
		}
		matchedRecord.Taker = *takerOrder
		matchedRecord.Maker = *makerOrder
		matchedRecord.MatchedRecordID = cast.ToString(idgen.NextId())
		matchedResult.MatchedRecords = append(matchedResult.MatchedRecords, matchedRecord)
	}
	matchedResult.MatchID = cast.ToString(idgen.NextId())
	//删除买盘中的被匹配完的订单，同时更新卖一价
	if len(deletedKeys) > 0 {
		for _, v := range deletedKeys {
			m.asks.orderBook.Remove(v)
		}
		m.updateBestAsk()
	}
	//更新深度数据
	for _, record := range matchedResult.MatchedRecords {
		p := &position{
			price: record.Price,
			qty:   record.Qty,
		}
		m.depthHandler.updateDepth(p, enum.Side_Sell, Delete, m.currentSeqId)
	}
	matchedResult.MatchTime = time.Now().UnixNano()
	//m.Next <- matchedResult
	if len(matchedResult.MatchedRecords) > 0 {
		m.SendMatchResult(matchedResult)
	}

	if takerOrder.OrderStatus != enum.OrderStatus_ALLFilled {
		r := &MatchResult{
			CancelResp: &CancelResp{
				CancelId: takerOrder.SequenceId,
				CoinId:   m.c.SymbolInfo.QuoteCoinID,
				Qty:      takerOrder.UnfilledAmount.String(),
				Uid:      takerOrder.Uid,
			},
		}
		//发送取消订单
		m.SendMatchResult(r)

	}
}

// 匹配限价买单
func (m *MatchEngine) matchLimitOrderBuy(takerOrder *Order) {
	matchedResult := &MatchResult{
		MatchedRecords: make([]*MatchedRecord, 0, 2),
		TakerIsBuy:     true,
	}
	//买单从卖盘中找
	iterator := m.asks.orderBook.Iterator()
	//待被删除的key
	deletedKeys := make([]*Key, 0, 2)
	for iterator.Next() {
		makerOrder := iterator.Value().(*Order)

		//订单全部成交退出，或者小于下一个订单的价格。不再循环匹配。
		if takerOrder.OrderStatus == enum.OrderStatus_ALLFilled || makerOrder.Price.GreaterThan(takerOrder.Price) {
			break
		}
		//计较价格
		result := takerOrder.UnfilledQty.Cmp(makerOrder.UnfilledQty)
		var matchedRecord *MatchedRecord
		switch {
		//吃完maker
		case result == 1:
			takerOrder.OrderStatus = enum.OrderStatus_PartFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			//taker减的金额不能以maker为准，要以taker下单的价格乘以数量为准
			//比如 maker卖 price222 qty1 taker 买 price333 qty 1  本次匹配taker扣除的金额为333 成交的金额是已maker 222 为准
			takerAmount := qty.Mul(takerOrder.Price)
			takerOrder.UnfilledQty = takerOrder.UnfilledQty.Sub(qty)
			takerOrder.UnfilledAmount = takerOrder.UnfilledAmount.Sub(takerAmount)
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
            takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量			
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{
				//订单的剩余数量
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))

		case result == 0:
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled

			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			takerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
            takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量			
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))
		case result == -1:
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			makerOrder.OrderStatus = enum.OrderStatus_PartFilled
			//更新订单的剩余数量
			qty := takerOrder.UnfilledQty
			//amount := takerOrder.UnfilledAmount
			takerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = makerOrder.UnfilledQty.Sub(qty)
			makerAmount := makerOrder.Price.Mul(qty)
			//成交的金额不能使用taker的金额,使用maker成交的数量乘以maker的价格 比如 maker卖 price222 qty2 taker 买 price333 qty 1
			//maker的未成交金额 减 222 *1 价格以maker为准
			//taker buy 111 1 maker sell 100 1
			makerOrder.UnfilledAmount = makerOrder.UnfilledAmount.Sub(makerAmount)
            takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量			
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(makerAmount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(makerAmount)
			matchedRecord = &MatchedRecord{
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: makerAmount,
			}
		}
		//加入到匹配的结果中
		matchedRecord.Taker = *takerOrder
		matchedRecord.Maker = *makerOrder
		matchedRecord.MatchedRecordID = cast.ToString(idgen.NextId())
		matchedResult.MatchedRecords = append(matchedResult.MatchedRecords, matchedRecord)

	}
	//删除卖盘被匹配过的订单，更新卖一价
	if len(deletedKeys) > 0 {
		for _, v := range deletedKeys {
			m.asks.orderBook.Remove(v)
		}
		m.updateBestAsk()
	}
	//如果taker还是部分匹配，将订单加入的买盘中
	if takerOrder.OrderStatus == enum.OrderStatus_PartFilled {
		m.addOrder(takerOrder)
		p := &position{
			price: takerOrder.Price,
			qty:   takerOrder.UnfilledQty,
		}
		m.depthHandler.updateDepth(p, enum.Side_Buy, Add, m.currentSeqId)
	}
	//更新深度数据
	for _, record := range matchedResult.MatchedRecords {
		p := &position{
			price: record.Price,
			qty:   record.Qty,
		}
		m.depthHandler.updateDepth(p, enum.Side_Sell, Delete, m.currentSeqId)

	}
	matchedResult.MatchTime = time.Now().UnixNano()
	matchedResult.MatchID = cast.ToString(idgen.NextId())
	//发送撮合结果
	m.SendMatchResult(matchedResult)

}

// 匹配限价卖单
func (m *MatchEngine) matchLimitOrderSell(takerOrder *Order) {
	matchedResult := &MatchResult{
		MatchedRecords: make([]*MatchedRecord, 0, 2),
		TakerIsBuy:     false,
	}
	//遍历买盘
	iterator := m.bids.orderBook.Iterator()
	var matchedRecord *MatchedRecord
	deletedKeys := make([]*Key, 0, 2)
	for iterator.Next() {
		makerOrder := iterator.Value().(*Order)
		//订单全部成交退出，或者小于下一个订单的价格。不再循环匹配。
		if takerOrder.OrderStatus == enum.OrderStatus_ALLFilled || takerOrder.Price.GreaterThan(makerOrder.Price) {
			break
		}

		result := takerOrder.UnfilledQty.Cmp(makerOrder.UnfilledQty)
		switch {
		case result == 1:
			takerOrder.OrderStatus = enum.OrderStatus_PartFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			//taker减的金额不能以maker为准，要以taker下单的价格乘以数量为准 比如 maker买 price444 qty1 taker 卖 price333 qty 2 本次匹配taker扣除的金额为333
			takerAmount := qty.Mul(takerOrder.Price)
			takerOrder.UnfilledQty = takerOrder.UnfilledQty.Sub(qty)
			takerOrder.UnfilledAmount = takerOrder.UnfilledAmount.Sub(takerAmount)
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec

			// makerOrder.FilledQty = qty
			// takerOrder.FilledQty = qty
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			matchedRecord = &MatchedRecord{
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))
		case result == 0:
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			makerOrder.OrderStatus = enum.OrderStatus_ALLFilled

			//更新订单的剩余数量
			qty := makerOrder.UnfilledQty
			amount := makerOrder.UnfilledAmount
			takerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量			
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(amount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(amount)
			//加入到撮合记录
			//卖单吃买单，以买单价格为准
			matchedRecord = &MatchedRecord{
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: amount,
			}
			//将key加入的集合中
			deletedKeys = append(deletedKeys, iterator.Key().(*Key))
		case result == -1:
			takerOrder.OrderStatus = enum.OrderStatus_ALLFilled
			makerOrder.OrderStatus = enum.OrderStatus_PartFilled

			//更新订单的剩余数量
			qty := takerOrder.UnfilledQty
			//amount := takerOrder.UnfilledAmount
			takerOrder.UnfilledQty = utils.DecimalZeroMaxPrec
			takerOrder.UnfilledAmount = utils.DecimalZeroMaxPrec
			makerOrder.UnfilledQty = makerOrder.UnfilledQty.Sub(qty)
			makerAmount := makerOrder.Price.Mul(qty)
			//成交的金额不能使用taker的金额
			//使用maker成交的数量乘以maker的价格
			makerOrder.UnfilledAmount = makerOrder.UnfilledAmount.Sub(makerAmount)
			takerOrder.FilledQty = takerOrder.FilledQty.Add(qty)  // 更新已成交数量
            makerOrder.FilledQty = makerOrder.FilledQty.Add(qty)  // 更新已成交数量
			takerOrder.FilledAmount = takerOrder.FilledAmount.Add(makerAmount)
			makerOrder.FilledAmount = makerOrder.FilledAmount.Add(makerAmount)
			matchedRecord = &MatchedRecord{
				Price:  makerOrder.Price,
				Qty:    qty,
				Amount: makerAmount,
			}
		}
		matchedRecord.Taker = *takerOrder
		matchedRecord.Maker = *makerOrder
		matchedRecord.MatchedRecordID = cast.ToString(idgen.NextId())
		matchedResult.MatchedRecords = append(matchedResult.MatchedRecords, matchedRecord)

	}
	//删除买盘被匹配过的订单，更新卖一价
	if len(deletedKeys) > 0 {
		for _, v := range deletedKeys {
			m.bids.orderBook.Remove(v)
		}
		m.updateBestBid()

	}
	//如果taker还是部分匹配，将订单加入的卖盘中
	if takerOrder.OrderStatus == enum.OrderStatus_PartFilled {

		m.addOrder(takerOrder)
		p := &position{
			price: takerOrder.Price,
			qty:   takerOrder.UnfilledQty,
		}
		m.depthHandler.updateDepth(p, enum.Side_Sell, Add, m.currentSeqId)
	}
	//更新深度数据
	for _, record := range matchedResult.MatchedRecords {
		p := &position{
			price: record.Price,
			qty:   record.Qty,
		}
		log.Printf("%+v", p.castToPosition(5, 6))
		m.depthHandler.updateDepth(p, enum.Side_Buy, Delete, m.currentSeqId)
	}
	matchedResult.MatchTime = time.Now().UnixNano()
	matchedResult.MatchID = cast.ToString(idgen.NextId())
	//m.Next <- matchedResult
	m.SendMatchResult(matchedResult)

}
func (m *MatchEngine) HandleOrder(order *Order) {

	//从接收输入的第一个订单开始，以后每次操作版本号加一
	if m.currentSeqId != 0 {
		m.currentSeqId = order.SequenceId
	} else {
		m.currentSeqId++
	}
	k := &Key{
		price: order.Price,
		id:    order.SequenceId,
	}
	var o interface{}
	var found bool
	if order.OrderType == enum.OrderType_LO {
		if order.Side == enum.Side_Sell {
			o, found = m.asks.orderBook.Get(k)
		} else {
			o, found = m.bids.orderBook.Get(k)
		}
	}
	//判断订单是否存在
	if (order.IsCancel && !found) || (!order.IsCancel && found) {
		return
	}

	// 1. 判断是否取消订单
	if order.IsCancel {
		orderDetail := o.(*Order)
		order.UnfilledQty = orderDetail.UnfilledQty
		order.UnfilledAmount = orderDetail.UnfilledAmount
		order.Amount = orderDetail.Amount
		order.Qty = orderDetail.Qty
		//订单簿删除订单
		m.cancelOrder(order)
		//更新盘口深度
		m.depthHandler.updateDepth(&position{
			price: order.Price,
			qty:   order.UnfilledQty,
		}, order.Side, Delete, m.currentSeqId)
		//发送取消订单消息

		coinId, qty := m.c.SymbolInfo.BaseCoinID, order.UnfilledQty.String()
		if orderDetail.Side == enum.Side_Buy {
			coinId = m.c.SymbolInfo.QuoteCoinID
			qty = order.UnfilledAmount.String()
		}
		m.SendMatchResult(&MatchResult{
			CancelResp: &CancelResp{
				CancelId: order.SequenceId,
				CoinId:   coinId,
				Qty:      qty,
				Uid:      orderDetail.Uid,
			},
			MatchTime: time.Now().UnixNano(),
		})
	} else {
		logx.Debugf("order = %+v bestBid = %v bestAsk=%v", order, m.bestBid, m.bestAsk)
		// 2. 根据订单类型和方向进行撮合
		switch {
		//买单市价单
		case order.Side == enum.Side_Buy && order.OrderType == enum.OrderType_MO:
			m.matchMarkerOrderBuy(order)  // 市价买单
		//买单限价单
		case order.Side == enum.Side_Buy && order.OrderType == enum.OrderType_LO:
			//价格大于卖一价，同时卖一价不为零
			if order.Price.GreaterThanOrEqual(m.bestAsk) && m.bestAsk.GreaterThan(utils.DecimalZeroMaxPrec) {
				m.matchLimitOrderBuy(order)   // 限价买单
			} else {
				m.addOrder(order)
				//更新盘口深度
				m.depthHandler.updateDepth(&position{
					price: order.Price,
					qty:   order.UnfilledQty,
				}, order.Side, Add, m.currentSeqId)
			}
		//卖单市价单
		case order.Side == enum.Side_Sell && order.OrderType == enum.OrderType_MO:
			m.matchMarketOrderSell(order) // 市价卖单
		//卖单限价单
		case order.Side == enum.Side_Sell && order.OrderType == enum.OrderType_LO:
			if order.Price.LessThanOrEqual(m.bestBid) && m.bestBid.GreaterThan(utils.DecimalZeroMaxPrec) {
				m.matchLimitOrderSell(order)  // 限价卖单
			} else {
				m.addOrder(order)
				//更新盘口深度
				m.depthHandler.updateDepth(&position{
					price: order.Price,
					qty:   order.UnfilledQty,
				}, order.Side, Add, m.currentSeqId)
			}
		}
		logx.Debugf(" bestBid = %v bestAsk=%v", m.bestBid, m.bestAsk)
	}

}
func (m *MatchEngine) GetDepth(level int32) DepthData {
	return m.depthHandler.getDepth(level)
}

// SendMatchResult 发送撮合结果，这个操作不异步。
/** 
使用消息队列实现异步解耦的主要场景是在撮合引擎中使用 Pulsar 消息队列实现撮合结果的异步处理。
主要解耦场景:
1.撮合引擎与账户资产更新解耦:
- 撮合引擎只负责撮合订单,将撮合结果发送到消息队列
- 账户服务异步消费消息队列,更新用户资产
2.撮合引擎与订单状态更新解耦:
- 撮合引擎完成撮合后不直接更新订单状态
- 订单服务异步消费消息队列,更新订单状态
3.消息幂等性保证:
- 使用 Redis 存储消息 ID 防止重复消费
- 不同服务使用不同前缀区分消息 ID
4.异步处理提升性能:
- 撮合引擎不需要等待资产和订单更新完成
- 多个消费者可以并行处理撮合结果
这种基于消息队列的异步解耦架构提高了系统的可扩展性和性能,同时保证了数据的最终一致性。
*/
func (m *MatchEngine) SendMatchResult(matchResult *MatchResult) {

	var resp matchMq.MatchResp
	resp.MessageId = stringx.Rand()
	if matchResult.CancelResp != nil {
		resp.Resp = &matchMq.MatchResp_Cancel{
			Cancel: &matchMq.CancelResp{
				Id:     matchResult.CancelResp.CancelId,
				CoinId: matchResult.CancelResp.CoinId,
				Qty:    matchResult.CancelResp.Qty,
				Uid:    matchResult.CancelResp.Uid,
			},
		}
	} else {
		beginPrice, endPrice := matchResult.MatchedRecords[0].Price.String(), matchResult.MatchedRecords[len(matchResult.MatchedRecords)-1].Price.String()
		lowPrice, highPrice := beginPrice, endPrice
		if !matchResult.TakerIsBuy {
			highPrice = beginPrice
			lowPrice = endPrice
		}
		records := make([]*matchMq.MatchResult_MatchedRecord, 0, len(matchResult.MatchedRecords))
		totalQty, totalAmount, takerUnFrozenAmount := utils.DecimalZeroMaxPrec, utils.DecimalZeroMaxPrec, utils.DecimalZeroMaxPrec
		for _, record := range matchResult.MatchedRecords {
			//本次撮合一共撮合了多少
			totalQty = totalQty.Add(record.Qty)
			totalAmount = totalAmount.Add(record.Amount)
			takerFilledQty := record.Taker.FilledQty.String()

			if record.Taker.OrderType == enum.OrderType_LO {
				//taker解冻的金额，以taker的成交价格为准
				a := record.Qty.Mul(record.Taker.Price)
				takerUnFrozenAmount = takerUnFrozenAmount.Add(a)
				takerFilledQty = record.Taker.Qty.Sub(record.Taker.UnfilledQty).String()

			} else {
				takerUnFrozenAmount = record.Taker.FilledAmount
			}
			makerFilledQty := record.Maker.Qty.Sub(record.Maker.UnfilledQty).String()
			r := &matchMq.MatchResult_MatchedRecord{
				Qty:        record.Qty.String(),
				Price:      record.Price.String(),
				Amount:     record.Amount.String(),
				MatchSubId: record.MatchedRecordID,
				Taker: &matchMq.OrderResp{
					OrderId:        record.Taker.OrderID,
					FilledQty:      takerFilledQty,
					UnFilledQty:    record.Taker.UnfilledQty.String(),
					FilledAmount:   record.Taker.FilledAmount.String(),
					UnFilledAmount: record.Taker.UnfilledAmount.String(),
					OrderStatus:    record.Taker.OrderStatus,
					Uid:            record.Taker.Uid,
					Id:             record.Taker.SequenceId,
					UnFrozenAmount: takerUnFrozenAmount.String(),
				},
				Maker: &matchMq.OrderResp{
					OrderId:        record.Maker.OrderID,
					FilledQty:      makerFilledQty,
					FilledAmount:   record.Maker.FilledAmount.String(),
					UnFilledQty:    record.Maker.UnfilledQty.String(),
					OrderStatus:    record.Maker.OrderStatus,
					UnFilledAmount: record.Maker.UnfilledAmount.String(),
					Uid:            record.Maker.Uid,
					Id:             record.Maker.SequenceId,
				},
			}
			records = append(records, r)
		}
		result := &matchMq.MatchResult{
			SymbolId:      m.c.SymbolInfo.SymbolID,
			SymbolName:    m.c.SymbolInfo.SymbolName,
			BaseCoinId:    m.c.SymbolInfo.BaseCoinID,
			QuoteCoinId:   m.c.SymbolInfo.QuoteCoinID,
			MatchId:       matchResult.MatchID,
			MatchedRecord: records,
			BeginPrice:    beginPrice,
			EndPrice:      endPrice,
			MatchTime:     matchResult.MatchTime,
			Qty:           totalQty.String(),
			Amount:        totalAmount.String(),
			HighPrice:     highPrice,
			LowPrice:      lowPrice,
			TakerIsBuy:    matchResult.TakerIsBuy,
		}
		resp.Resp = &matchMq.MatchResp_MatchResult{
			MatchResult: result,
		}
	}

	logx.Debugw("send match result", logx.Field("data", &resp))
	data, _ := proto.Marshal(&resp)
	var err error
	// 1. 撮合引擎发送撮合结果：撮合引擎通过 Pulsar Producer 将撮合结果发送到消息队列
	for i := 0; i < 10; i++ {
		if _, err = m.producer.Send(context.Background(), &pulsar.ProducerMessage{
			Payload: data,
		}); err != nil {
			logx.Errorw("send message failed", logger.ErrorField(err), logx.Field("count", i+1))
			time.Sleep(time.Second)
			continue
		}
		break
	}
	if err != nil {
		logx.Severef("send message failed err=%v", err)
	}

	m.tick <- matchResult
}

// 这是一个偷懒的写法，我觉得放到matchmq会比较好。
func (m *MatchEngine) sendTick() {
	for matchResult := range m.tick {
		for _, v := range matchResult.MatchedRecords {
			tick := commonWs.Tick{
				Price:        v.Price.StringFixedBank(m.c.SymbolInfo.QuoteCoinPrec.Load()),
				Qty:          v.Qty.StringFixedBank(m.c.SymbolInfo.BaseCoinPrec.Load()),
				Amount:       v.Amount.StringFixedBank(m.c.SymbolInfo.QuoteCoinPrec.Load()),
				TimeStamp:    matchResult.MatchTime / 1e9,
				TakerIsBuyer: matchResult.TakerIsBuy,
			}
			msg := commonWs.Message[commonWs.Tick]{
				Topic:   commonWs.TickPrefix.WithParam(m.c.SymbolInfo.SymbolName),
				Payload: tick,
			}
			if _, err := m.proxyClient.PushData(context.Background(), &ws.Data{
				Uid:   "",
				Topic: commonWs.TickPrefix.WithParam(m.c.SymbolInfo.SymbolName),
				Data:  msg.ToBytes(),
			}); err != nil {
				logx.Errorw("push kline websocket data failed", logger.ErrorField(err), logx.Field("data", tick))
			}
		}
	}
}
