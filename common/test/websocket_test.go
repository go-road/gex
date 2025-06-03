package test

import (
	"context"
	"testing"
	"time"

	"github.com/luxun9527/gex/common/ws/socket"
)

// go test -v -run TestWebSocket ./common/test/websocket_test.go
// tail -f /var/log/websocket/websocket.log
// netstat -tulpn | grep 9992
// netstat -tulnp | grep 9992
// telnet localhost 9992
// curl -i -N -H "Connection: Upgrade" -H "Upgrade: websocket" -H "Host: localhost" -H "Origin: http://localhost" http://localhost:9992/ws

// curl -i -N \
//   -H "Connection: Upgrade" \
//   -H "Upgrade: websocket" \
//   -H "Host: localhost" \
//   -H "Origin: http://localhost" \
//   http://localhost:9992/ws

// # 从容器内部测试(如果服务绑定地址未显式指定为0.0.0.0可能telnet会失败)
// docker exec -it ws_socket sh
// telnet localhost 9992

// # 测试容器间连通性
// docker exec -it ws_socket ping accountapi
// docker exec -it ws_socket curl -v http://accountapi:20014/account/v1/validate_token

// # 容器内部测试
// docker exec -it ws_socket wget -O- http://localhost:9992/stats
// # 返回包含连接数的JSON数据
// docker exec ws_socket wget -qO- http://localhost:9992/stats

// docker logs ws_socket | grep "test_channel"

func TestWebSocket(t *testing.T) {
	// proxy_url := "172.23.0.12:10067/ws"
	// url := "ws://192.168.1.4/ws"
	url := "ws://localhost:9992/ws"
	
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySUQiOjMsIlVzZXJuYW1lIjoibGlzaSIsIk5pY2tOYW1lIjoiIiwiaXNzIjoiemhhbmdzYW4iLCJhdWQiOlsiR1ZBIl0sImV4cCI6MTc0OTUyMDIxMCwibmJmIjoxNzQ4NjU2MjEwfQ.Z4E9kDH-IWq3LcdshYAeM80d-XXM3ZNVDLwtqOYK9U4"
	uid := "3"
	
	// Create client with debug logging
	client := socket.NewClient(url, token, uid)
	
	// Set connection timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Connect with context
	err := client.ConnectWithContext(ctx)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()
	
	// Send test message
	// {"code":1,"topic":"kline@IKUN_USDT@Day1"}
	// {"code":5,"topic":"order@IKUN_USDT"}
	// {"code":1,"topic":"ticker@IKUN_USDT"}
	topic:= "kline@IKUN_USDT@Day1"
	topic = "order@IKUN_USDT"
	topic = "ticker@IKUN_USDT"
	sendMsg := []byte(`{
		"code": 1,
		"topic": "` + topic + `",
		"payload": "hello"
	}`)
	if err := client.Write(sendMsg); err != nil {
		t.Errorf("Failed to write message: %v", err)
	}
	
	// Wait for response
	time.Sleep(2 * time.Second)

	// 在客户端添加心跳
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := client.Write([]byte(`{"code":0}`)); err != nil {
					return
				}
			}
		}
	}()

	// 断线重连
	for {
		err := client.ConnectWithContext(ctx)
		if err != nil {
			t.Logf("连接失败，10秒后重试...")
			time.Sleep(10 * time.Second)
			continue
		}
		break
	}

	// 持续监听消息
    go func() {
        for {
            msg, err := client.Read()
            if err != nil {
                t.Logf("读取消息错误: %v", err)
                return
            }
            t.Logf("收到消息: %s", msg)
        }
    }()

    // 保持连接不退出
    select {}  // 永久阻塞
}