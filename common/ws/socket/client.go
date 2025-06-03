package socket

import (
	// "context"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeromicro/go-zero/core/logx"
)

/**
主要实现了以下功能:
1.创建WebSocket连接
2.处理消息收发
3.实现重连机制
4.处理认证

1.Client结构体:
	conn: WebSocket连接
	writeChan: 消息发送通道
	closeChan: 关闭通道
	token/uid: 认证信息
	url: WebSocket服务地址
	mu: 互斥锁保护并发访问
2.主要方法:
	Connect(): 建立WebSocket连接,设置认证头
	readLoop(): 持续读取消息
	writeLoop(): 发送消息和心跳
	reconnect(): 断线重连
	Close(): 关闭连接
	Write(): 发送消息
	handleMessage(): 处理接收到的消息
3.特性:
	自动重连
	心跳保活
	并发安全
	消息缓冲
	认证支持
	消息分发处理
*/

// 一个健壮的WebSocket客户端,可以:
// 安全地处理并发
// 自动重连
// 支持认证
// 处理不同类型的消息
// 心跳保活

type Client struct {
	conn      *websocket.Conn
	readChan  chan []byte
	writeChan chan []byte
	closeChan chan struct{}
	once      sync.Once
	token     string
	uid       string
	url       string
	mu        sync.Mutex
}
func NewClient(url, token, uid string) *Client {
	return &Client{
		readChan:  make(chan []byte, 100),
		writeChan: make(chan []byte, 100),
		closeChan: make(chan struct{}),
		token:     token,
		uid:       uid,
		url:       url,
	}
}
func (c *Client) Connect() error {
	// 设置请求头
	header := http.Header{}
	header.Set("gexToken", c.token)
	header.Set("gexUserId", c.uid)
	// 建立WebSocket连接
	conn, _, err := websocket.DefaultDialer.Dial(c.url, header)
	if err != nil {
		return fmt.Errorf("websocket dial error: %v", err)
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	// 启动读写goroutine
	go c.readLoop()
	go c.writeLoop()
	return nil
}
func (c *Client) ConnectWithContext(ctx context.Context) error {
    // 设置请求头
    header := http.Header{}
    header.Set("gexToken", c.token)
    header.Set("gexUserId", c.uid)
    
    // 配置WebSocket dialer
    dialer := websocket.Dialer{
        HandshakeTimeout: 10 * time.Second,
        // 增加TLS配置
        TLSClientConfig: &tls.Config{
            InsecureSkipVerify: true,
        },
    }
    // 重试连接逻辑
    var conn *websocket.Conn
    var resp *http.Response
    var err error
    
    for retries := 0; retries < 3; retries++ {
        conn, resp, err = dialer.DialContext(ctx, c.url, header)
        if err == nil {
            break
        }
        
        if resp != nil {
            logx.Errorf("websocket connection failed: status=%d, retry=%d, err=%v", 
                resp.StatusCode, retries, err)
        } else {
            logx.Errorf("websocket connection failed: retry=%d, err=%v", retries, err)
        }
        
        // 指数退避重试
        time.Sleep(time.Duration(1<<uint(retries)) * time.Second)
    }
    
    if err != nil {
        return fmt.Errorf("websocket connection failed after retries: %v", err)
    }
    c.mu.Lock()
    c.conn = conn
    c.mu.Unlock()
    // 启动读写循环
    go c.readLoop()
    go c.writeLoop()
    
    return nil
}
func (c *Client) readLoop() {
	for {
		select {
		case <-c.closeChan:
			return
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				logx.Errorf("read message error: %v", err)
				c.reconnect()
				return
			}
			// 处理接收到的消息
			c.handleMessage(message)
		}
	}
}
func (c *Client) writeLoop() {
    ticker := time.NewTicker(time.Second * 30)
    defer ticker.Stop()
    
    for {
        select {
        case <-c.closeChan:
            return
        case message := <-c.writeChan:
            c.mu.Lock()
            err := c.conn.WriteMessage(websocket.TextMessage, message)
            c.mu.Unlock()
            if err != nil {
                logx.Errorf("write message error: %v", err)
                c.reconnect()
                return
            }
        case <-ticker.C:
            c.mu.Lock()
            // 发送ping消息
            if err := c.conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
                logx.Errorf("ping error: %v", err)
                c.mu.Unlock()
                c.reconnect()
                return
            }
            c.mu.Unlock()
        }
    }
}
func (c *Client) reconnect() {
    c.mu.Lock()
    if c.conn != nil {
        c.conn.Close()
        c.conn = nil
    }
    c.mu.Unlock()
    // 重连间隔
    time.Sleep(time.Second * 2)
    // 使用context with timeout重新连接
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    if err := c.ConnectWithContext(ctx); err != nil {
        logx.Errorf("reconnect failed: %v", err)
    }
}
// func (c *Client) reconnect() {
// 	c.mu.Lock()
// 	defer c.mu.Unlock()
// 	// 关闭旧连接
// 	if c.conn != nil {
// 		c.conn.Close()
// 	}
// 	// 重新连接
// 	for {
// 		err := c.Connect()
// 		if err == nil {
// 			break
// 		}
// 		logx.Errorf("reconnect error: %v", err)
// 		time.Sleep(time.Second * 5)
// 	}
// }
func (c *Client) Close() {
	c.once.Do(func() {
		close(c.closeChan)
		if c.conn != nil {
			c.conn.Close()
		}
	})
}
func (c *Client) Read() ([]byte, error) {
    select {
    case msg := <-c.readChan:
        return msg, nil
    case <-c.closeChan:
        return nil, fmt.Errorf("connection closed")
    }
}
func (c *Client) Write(message []byte) error {
	select {
	case c.writeChan <- message:
		return nil
	default:
		logx.Error("write channel full")
		return fmt.Errorf("write channel full, message dropped")
	}
}
func (c *Client) handleMessage(message []byte) {
	// 解析并处理消息
	var msg struct {
		Topic   string          `json:"topic"`
		Payload json.RawMessage `json:"payload"` 
	}
	
	if err := json.Unmarshal(message, &msg); err != nil {
		logx.Errorf("unmarshal message error: %v", err)
		return
	}
	// 根据topic处理不同类型的消息
	logx.Infof("receive message: %v", msg)
	switch msg.Topic {
	case "depth":
		// 处理深度数据
	case "ticker": 
		// 处理行情数据
	case "kline":
		// 处理K线数据
	default:
		logx.Infof("unknown topic: %s", msg.Topic)
	}
	// 将消息发送到读取通道
    select {
    case c.readChan <- message:
    default:
        logx.Error("read channel full, message dropped")
    }
}