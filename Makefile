
genaccount:
#gormt 通过数据库生成指定的结构体 https://github.com/xxjwxc/gormt -z config.yaml 指定配置文件路径
	gentool --dsn="root:root@tcp(192.168.1.4:3307)/trade?charset=utf8mb4&parseTime=True&loc=Local" --db=mysql --tables=user,asset  -outPath=app/account/rpc/internal/dao/query -fieldMap="decimal:string;tinyint:int32;"
accountapi:
	   goctl api go -api=app/account/api/desc/account.api -dir=app/account/api -style=go_zero  -home=template && make accountdoc
accountdoc:
	   goctl api plugin -plugin goctl-swagger="swagger -filename doc/account.json -host api.gex.com" -api app/account/api/desc/account.api -dir .
accountrpc:
	   goctl rpc  protoc app/account/rpc/pb/account.proto --go_out=app/account/rpc --go-grpc_out=app/account/rpc   --zrpc_out=app/account/rpc -style=go_zero  -home=template
orderrpc:
	   goctl rpc  protoc -Icommon/proto -I./ app/order/rpc/pb/order.proto --go_out=app/order/rpc --go-grpc_out=app/order/rpc   --zrpc_out=app/order/rpc -style=go_zero  -home=template
orderapi:
	   goctl api go -api=app/order/api/desc/order.api -dir=app/order/api -style=go_zero  -home=template && make orderdoc
orderdoc:
	   goctl api plugin -plugin goctl-swagger="swagger -filename doc/order.json -host api.gex.com" -api app/order/api/desc/order.api -dir .
genorder:
	gentool --dsn="root:root@tcp(192.168.1.4:3307)/trade?charset=utf8mb4&parseTime=True&loc=Local" --db=mysql --tables=entrust_order_00,matched_order  -outPath=app/order/rpc/internal/dao/query -fieldMap="decimal:string;tinyint:int32;bigint:int64;"
enum:
	protoc   -I. --go_out=./  common/proto/enum/*.proto
matchmq:
	#--go_out指定的路径和option go_package = "trade/common/proto/mq/match;proto"; 指定的路径一起决定文件生成的位置 这个路径trade/common/proto/mq/match也是别人导入时用到的路径。
	protoc    -Icommon/proto -I./ --go_out=./ common/proto/mq/match/match.proto && make matchmodel
matchrpc:
	goctl rpc  protoc -I./ -Icommon/proto app/match/rpc/pb/match.proto --go_out=app/match/rpc --go-grpc_out=app/match/rpc   --zrpc_out=app/match/rpc -style=go_zero  -home=template
	make matchmodel
matchmodel:
	gentool --dsn="root:root@tcp(192.168.1.4:3307)/trade?charset=utf8mb4&parseTime=True&loc=Local" --db=mysql --tables=matched_order  -outPath=app/match/rpc/internal/dao/query -fieldMap="decimal:string;tinyint:int32;int:int64"
klinerpc:
	goctl rpc  protoc -I./ app/quotes/kline/rpc/pb/kline.proto --go_out=app/quotes/kline/rpc --go-grpc_out=app/quotes/kline/rpc  --zrpc_out=app/quotes/kline/rpc -style=go_zero  -home=template
tickerrpc:
	goctl rpc  protoc -I./ app/quotes/ticker/rpc/pb/ticker.proto --go_out=app/quotes/ticker/rpc --go-grpc_out=app/quotes/ticker/rpc  --zrpc_out=app/quotes/ticker/rpc -style=go_zero  -home=template
depthrpc:
	goctl rpc  protoc -I./ app/quotes/depth/rpc/pb/depth.proto --go_out=app/quotes/depth/rpc --go-grpc_out=app/quotes/depth/rpc  --zrpc_out=app/quotes/depth/rpc -style=go_zero  -home=template

klinemodel:
	gentool --dsn="root:root@tcp(192.168.1.4:3307)/trade?charset=utf8mb4&parseTime=True&loc=Local" --db=mysql --tables=kline  -outPath=app/quotes/kline/rpc/internal/dao/query -fieldMap="decimal:string;tinyint:int32;int:int64"
quoteapi:
	   goctl api go -api=app/quotes/api/desc/quotes.api -dir=app/quotes/api -style=go_zero  -home=template && make quotedoc
quotedoc:
	goctl api plugin -plugin goctl-swagger="swagger -filename doc/quotes.json -host api.gex.com" -api app/quotes/quotes-api/desc/quotes.api -dir .

adminapi:
	goctl api go -api=app/admin/api/desc/admin.api -dir=app/admin/api -style=go_zero  -home=template &&   make admindoc

admindoc:
	goctl api plugin -plugin goctl-swagger="swagger -filename doc/admin.json -host api.gex.com" -api app/admin/api/desc/admin.api -dir .

adminmodel:
	gentool --dsn="root:root@tcp(192.168.1.4:3307)/admin?charset=utf8mb4&parseTime=True&loc=Local" --db=mysql  -outPath=app/admin/api/internal/dao/admin/query -fieldMap="decimal:string;tinyint:int32;int:int32" -fieldSignable=true
	softdeleted -p app/admin/api/internal/dao/model/*.go
	gentool --dsn="root:root@tcp(192.168.1.4:3307)/trade?charset=utf8mb4&parseTime=True&loc=Local" --db=mysql --tables=matched_order  -outPath=app/admin/api/internal/dao/match/query -fieldMap="decimal:string;tinyint:int32;int:int64"

kline:
	make klinerpc  && make klinemodel

# Starts both infrastructure and application containers
run:
	make pre
	chmod +x ./deploy/scripts/run.sh
	sudo rm -rf deploy/depend/pulsar/data/*	
	./deploy/scripts/run.sh
		
up:
	docker compose -f deploy/depend/docker-compose.yaml up -d
	docker compose -f deploy/dockerfiles/docker-compose.yaml up -d

start:
	docker compose -f deploy/depend/docker-compose.yaml start
	docker compose -f deploy/dockerfiles/docker-compose.yaml start

down:
	docker compose -f deploy/dockerfiles/docker-compose.yaml down
	docker compose -f deploy/depend/docker-compose.yaml down	

stop:
	docker compose -f deploy/dockerfiles/docker-compose.yaml stop
	docker compose -f deploy/depend/docker-compose.yaml stop

clear:
	chmod +x ./deploy/scripts/remove_containers.sh
	chmod +x ./deploy/scripts/remove_images.sh
	./deploy/scripts/remove_containers.sh
	./deploy/scripts/remove_images.sh
	rm -rf deploy/depend/mysql/data/*

pre:
	chmod +x ./bin/accountapi
	chmod +x ./bin/accountrpc
	chmod +x ./bin/adminapi
	chmod +x ./bin/matchmq
	chmod +x ./bin/matchrpc
	chmod +x ./bin/orderapi
	chmod +x ./bin/orderrpc
	chmod +x ./bin/quoteapi
	chmod +x ./bin/klinerpc
	chmod +x ./deploy/depend/dtm/dtm
	chmod +x ./deploy/depend/ws/proxy/proxy
	chmod +x ./deploy/depend/ws/socket/socket

dep1:
	docker compose -f deploy/depend/docker-compose.yaml up
dep2:
	docker compose -f deploy/dockerfiles/docker-compose.yaml up

build:
	go env -w GOOS=linux
	go env -w  GOPROXY=https://goproxy.cn,direct
	go env -w  CGO_ENABLED=0
	go build  -ldflags="-s -w"  -o ./bin/accountapi ./app/account/api/account.go
	go build -ldflags="-s -w" -o ./bin/accountrpc ./app/account/rpc/account.go
	go build -ldflags="-s -w" -o ./bin/adminapi ./app/admin/api/admin.go
	go build -ldflags="-s -w" -o ./bin/matchmq ./app/match/mq/match.go
	go build -ldflags="-s -w" -o ./bin/matchrpc ./app/match/rpc/match.go
	go build -ldflags="-s -w" -o ./bin/orderapi ./app/order/api/order.go
	go build -ldflags="-s -w" -o ./bin/orderrpc ./app/order/rpc/order.go
	go build -ldflags="-s -w" -o ./bin/quoteapi ./app/quotes/api/quote.go
	go build -ldflags="-s -w" -o ./bin/klinerpc ./app/quotes/kline/rpc/kline.go

rebuild-accountapi:
	go build  -ldflags="-s -w"  -o ./bin/accountapi ./app/account/api/account.go
	# docker compose -f deploy/dockerfiles/docker-compose.yaml build accountapi
	docker compose -f deploy/dockerfiles/docker-compose.yaml up -d --build --force-recreate accountapi
	docker logs -f accountapi

# 编译二进制文件=>	启动容器（含自动构建镜像并停止、删除原来的容器后强制重新创建新的实例）=> 查看容器日志
rebuild-accountrpc: 
	# docker stop accountrpc && docker rm accountrpc
	go build -ldflags="-s -w" -o ./bin/accountrpc ./app/account/rpc/account.go
	# docker compose -f deploy/dockerfiles/docker-compose.yaml build accountrpc
	docker compose -f deploy/dockerfiles/docker-compose.yaml up -d --build --force-recreate accountrpc
	docker logs -f accountrpc

rebuild-pulsar:
	docker compose -f deploy/depend/docker-compose.yaml down pulsar
	sudo rm -rf deploy/depend/pulsar/data/*	
	./deploy/scripts/run.sh
	docker logs pulsar -f

rebuild-ws:
	docker compose -f deploy/depend/docker-compose.yaml up -d --force-recreate wssocket	
	docker exec -it ws_socket netstat -tulnp | grep 9992

# Starts both infrastructure containers and application services locally
## 启动基础设施容器
run-infra:
	chmod +x ./deploy/scripts/run_infra.sh
	sudo rm -rf deploy/depend/pulsar/data/*	
	./deploy/scripts/run_infra.sh

## 启动本地服务(使用本地配置文件)
run-local:
	go env -w GOOS=linux
	go env -w  GOPROXY=https://goproxy.cn,direct
	go env -w  CGO_ENABLED=0
### 启动本地 RPC 服务
	go run app/account/rpc/account.go -f app/account/rpc/etc/account_local_20022.yaml &
	go run app/order/rpc/order.go -f app/order/rpc/etc/order_local_20027.yaml &
	go run app/quotes/kline/rpc/kline.go -f app/quotes/kline/rpc/etc/kline_local_20029.yaml &
### 启动本地 API 服务	
	go run app/account/api/account.go -f app/account/api/etc/account_local_20024.yaml &
	go run app/match/mq/match.go -f app/match/mq/etc/match.yaml &
	go run app/admin/api/admin.go -f app/admin/api/etc/admin_local_20025.yaml &
	go run app/order/api/order.go -f app/order/api/etc/order_local_20026.yaml &
	go run app/quotes/api/quote.go -f app/quotes/api/etc/quote_local_20021.yaml &
### 最后启动本地 Match RPC 服务
	sleep 10
	go run app/match/rpc/match.go -f app/match/rpc/etc/match_local_20023.yaml &


# 查看服务日志	
logs:
	@echo "=== Account RPC Logs ==="
	@ps aux | grep "[g]o run app/account/rpc/account.go" | awk '{print $2}' | xargs -I {} tail -f /proc/{}/fd/1 &
	@echo "=== Account API Logs ==="
	@ps aux | grep "[g]o run app/account/api/account.go" | awk '{print $2}' | xargs -I {} tail -f /proc/{}/fd/1 &
	@echo "=== Order RPC Logs ==="
	@ps aux | grep "[g]o run app/order/rpc/order.go" | awk '{print $2}' | xargs -I {} tail -f /proc/{}/fd/1 &
### Add more services as needed

## 启动本地服务(使用本地配置文件)-备用方案一
## 使用 gnome-terminal 在新终端窗口中启动每个服务，需要系统安装了 gnome-terminal
## sudo apt-get install gnome-terminal
## 添加 exec bash 保持终端窗口打开以查看日志
## 使用 & 符号使命令在后台运行
run-local-gnome-terminal:
	go env -w GOOS=linux
	go env -w  GOPROXY=https://goproxy.cn,direct
	go env -w  CGO_ENABLED=0
### 启动本地 RPC 服务
	gnome-terminal --tab --title="AccountRPC" -- bash -c "go run app/account/rpc/account.go -f app/account/rpc/etc/account_local_20022.yaml; exec bash" &
	gnome-terminal --tab --title="MatchRPC" -- bash -c "go run app/match/rpc/match.go -f app/match/rpc/etc/match_local_20023.yaml; exec bash" &
	gnome-terminal --tab --title="OrderRPC" -- bash -c "go run app/order/rpc/order.go -f app/order/rpc/etc/order_local_20027.yaml; exec bash" &
	gnome-terminal --tab --title="KlineRPC" -- bash -c "go run app/quotes/kline/rpc/kline.go -f app/quotes/kline/rpc/etc/kline_local_20029.yaml; exec bash" &
### 启动本地 API 服务	
	gnome-terminal --tab --title="AccountAPI" -- bash -c "go run app/account/api/account.go -f app/account/api/etc/account_local_20024.yaml; exec bash" &
	gnome-terminal --tab --title="MatchMQ" -- bash -c "go run app/match/mq/match.go -f app/match/mq/etc/match.yaml; exec bash" &
	gnome-terminal --tab --title="AdminAPI" -- bash -c "go run app/admin/api/admin.go -f app/admin/api/etc/admin_local_20025.yaml; exec bash" &
	gnome-terminal --tab --title="OrderAPI" -- bash -c "go run app/order/api/order.go -f app/order/api/etc/order_local_20026.yaml; exec bash" &
	gnome-terminal --tab --title="QuoteAPI" -- bash -c "go run app/quotes/api/quote.go -f app/quotes/api/etc/quote_local_20021.yaml; exec bash" &

## 启动本地服务(使用本地配置文件)-备用方案二
## 使用 gnome-terminal 在新终端窗口中启动每个服务，需要系统安装了 gnome-terminal
run-local-logs:
	go env -w GOOS=linux
	go env -w  GOPROXY=https://goproxy.cn,direct
	go env -w  CGO_ENABLED=0
### 启动本地 RPC 服务
	mkdir -p logs
	gnome-terminal --tab --title="AccountRPC" -- bash -c "go run app/account/rpc/account.go -f app/account/rpc/etc/account_local_20022.yaml 2>&1 | tee logs/account_rpc.log; exec bash" &
	gnome-terminal --tab --title="MatchRPC" -- bash -c "go run app/match/rpc/match.go -f app/match/rpc/etc/match_local_20023.yaml 2>&1 | tee logs/match_rpc.log; exec bash" &
	gnome-terminal --tab --title="OrderRPC" -- bash -c "go run app/order/rpc/order.go -f app/order/rpc/etc/order_local_20027.yaml 2>&1 | tee logs/order_rpc.log; exec bash" &
	gnome-terminal --tab --title="KlineRPC" -- bash -c "go run app/quotes/kline/rpc/kline.go -f app/quotes/kline/rpc/etc/kline_local_20029.yaml 2>&1 | tee logs/kline_rpc.log; exec bash" &
### 启动本地 API 服务	
	gnome-terminal --tab --title="AccountAPI" -- bash -c "go run app/account/api/account.go -f app/account/api/etc/account_local_20024.yaml 2>&1 | tee logs/account_api.log; exec bash" &
	gnome-terminal --tab --title="MatchMQ" -- bash -c "go run app/match/mq/match.go -f app/match/mq/etc/match.yaml 2>&1 | tee logs/match_mq.log; exec bash" &
	gnome-terminal --tab --title="AdminAPI" -- bash -c "go run app/admin/api/admin.go -f app/admin/api/etc/admin_local_20025.yaml 2>&1 | tee logs/admin_api.log; exec bash" &
	gnome-terminal --tab --title="OrderAPI" -- bash -c "go run app/order/api/order.go -f app/order/api/etc/order_local_20026.yaml 2>&1 | tee logs/order_api.log; exec bash" &
	gnome-terminal --tab --title="QuoteAPI" -- bash -c "go run app/quotes/api/quote.go -f app/quotes/api/etc/quote_local_20021.yaml 2>&1 | tee logs/quote_api.log; exec bash" &

## 启动本地服务(使用本地配置文件)-备用方案三
run-local-service:
	go env -w GOOS=linux
	go env -w  GOPROXY=https://goproxy.cn,direct
	go env -w  CGO_ENABLED=0
### 启动本地 RPC 服务
	go run app/account/rpc/account.go -f app/account/rpc/etc/account_local_20022.yaml &
	go run app/match/rpc/match.go     -f app/match/rpc/etc/match_local_20023.yaml &
	go run app/order/rpc/order.go     -f app/order/rpc/etc/order_local_20027.yaml &
	go run app/quotes/kline/rpc/kline.go  -f app/quotes/kline/rpc/etc/kline_local_20029.yaml &
### 启动本地 API 服务	
	go run app/account/api/account.go -f app/account/api/etc/account_local_20024.yaml &
	go run app/match/mq/match.go      -f app/match/mq/etc/match.yaml &
	go run app/admin/api/admin.go     -f app/admin/api/etc/admin_local_20025.yaml &
	go run app/order/api/order.go     -f app/order/api/etc/order_local_20026.yaml &
	go run app/quotes/api/quote.go 	  -f app/quotes/api/etc/quote_local_20021.yaml &

## 启动本地服务(使用本地配置文件)-备用方案四
run-local-logfile:
	go env -w GOOS=linux
	go env -w  GOPROXY=https://goproxy.cn,direct
	go env -w  CGO_ENABLED=0
### 启动本地 RPC 服务
	@echo "Starting RPC services..."
	go run app/account/rpc/account.go -f app/account/rpc/etc/account_local_20022.yaml > logs/account_rpc.log 2>&1 &
	go run app/match/rpc/match.go -f app/match/rpc/etc/match_local_20023.yaml > logs/match_rpc.log 2>&1 &
	go run app/order/rpc/order.go -f app/order/rpc/etc/order_local_20027.yaml > logs/order_rpc.log 2>&1 &
	go run app/quotes/kline/rpc/kline.go -f app/quotes/kline/rpc/etc/kline_local_20029.yaml > logs/kline_rpc.log 2>&1 &
### 启动本地 API 服务	
	@echo "Starting API services..."
	go run app/account/api/account.go -f app/account/api/etc/account_local_20024.yaml > logs/account_api.log 2>&1 &
	go run app/match/mq/match.go -f app/match/mq/etc/match.yaml > logs/match_mq.log 2>&1 &
	go run app/admin/api/admin.go -f app/admin/api/etc/admin_local_20025.yaml > logs/admin_api.log 2>&1 &
	go run app/order/api/order.go -f app/order/api/etc/order_local_20026.yaml > logs/order_api.log 2>&1 &
	go run app/quotes/api/quote.go -f app/quotes/api/etc/quote_local_20021.yaml > logs/quote_api.log 2>&1 &
	@echo "All services started. Check logs in logs/ directory"

# Stop all locally running services
# 停止所有本地运行的服务

# 使用 pid 文件管理：
# 如果条件允许，考虑在每个服务启动时记录各自的 pid 到指定的 pid 文件里，然后在 stop-local 中读取 pid 文件后使用 kill 命令直接退出对应的进程。这样可以避免使用 pkill 的模糊匹配问题，更精确地控制进程。

# 杀死多个进程
# ps aux | grep -E 'account_local_|match_local_|match.yaml|order_local_|kline_local_|admin_local_|quote_local_' | awk '{print $2}' | xargs kill -9
# ps aux | egrep   'account_local_|match_local_|match.yaml|order_local_|kline_local_|admin_local_|quote_local_' | awk '{print $2}' | xargs kill -9
# ps aux | grep "[g]o run app" | awk '{print $2}' | xargs kill -9
# 命令说明：
# 使用 grep -E '...' 后，所有关键词都会被作为正则表达式匹配（注意这时“|”表示或的关系）。
# 注意：ps aux 输出的第二列才是PID（第一列通常是用户名），所以建议用 awk '{print $2}' 来提取PID，如果想杀掉进程（当然有时也可能选第一列，但一般需要PID）。

# 杀死单个进程
# 1. 查找包含 "go run app/account/rpc/account.go" 的进程
# ps aux | grep "[g]o run app/account/rpc/account.go"
# 2. 查找并直接杀死进程(推荐使用这个)
# ps aux | grep "[g]o run app/account/rpc/account.go" | awk '{print $2}' | xargs kill -9
# 3. 如果要杀死所有本地运行的服务进程
# ps aux | grep "[g]o run app" | awk '{print $2}' | xargs kill -9
# 命令说明:
# ps aux - 显示所有进程信息
# grep "[g]o run ..." - 查找包含指定命令的进程。使用[g]避免grep自身进程
# awk '{print $2}' - 提取第2列(PID)
# xargs kill -9 - 将PID传给kill命令强制终止进程

# 使用 pkill 命令停止所有通过 go run 启动的服务
# 注意 Makefile 中每条命令默认在独立的 shell 中执行，但如果其中一条命令的退出码不为 0，make 会认为发生错误，从而中止整个目标，可以确保所有 命令都加上 "|| true" 来忽略错误。
# 在 Makefile 中设置 “-” 前缀可以忽略错误，这样即使返回非零，也不会中断整个目标。
# pkill 命令正常会返回被杀进程的数量，如果没有进程被匹配到，也可能返回非 0 值，所以加 "|| true" 或使用统一退出码是较好的做法。
# sudo apt update
# sudo apt-get install procps
# pkill -f "go run app"
# pkill -f "go run app/account/rpc/account.go"

# 命令说明：
# 使用 [g]o 替换 go 可以确保目标进程命令行中包含 go run app/account/rpc/account.go 依然能匹配，但不会匹配包含字面值 "[g]o run app/account/rpc/account.go" 的当前命令行（由于自身的命令行并不包含这一格式的字符串）。
# 在每个 pkill 命令后添加 "|| true" 以保证即使命令未找到匹配进程也不会导致非零退出码影响整个目标的执行。
# 使用换行符（反斜杠）让多个命令在同一个 sh -c 中执行，最后用 exit 0 结束。
# 这样修改后，停止服务时就不会误杀正在执行的 shell 进程，也不会导致 make 被中断。

# 1、进程未杀干净的现象分析：
# Makefile 中的 pkill 命令使用的是匹配字符串，比如：
# pkill -e -f "go run app/account/rpc/account.go"
# 这条命令会查找包含 "go run app/account/rpc/account.go" 的进程并杀掉。但观察到的进程却是类似于 “/tmp/go-build.../exe/account -f app/account/rpc/etc/account_local_20022.yaml” 这样的进程。

# 2、原因在于：  
# 使用 "go run ..." 启动进程时，进程命令行会包含 "go run ..." 字样，这样 pkill 能匹配；  
# 而当编译后运行（比如从 /tmp/go-build/... 目录下的可执行文件运行）时，命令行中不再包含 "go run ..."，而是只包含可执行文件的实际路径（如 /tmp/go-build…/exe/account ...）。因此，原来的匹配模式无法匹配到这些进程。
# 解决方法有两个方向：

# 3、【方案1：调整 pkill 匹配模式】
# 可以修改 Makefile 中杀进程的模式，既杀掉使用 “go run” 启动的进程，也杀掉通过编译生成的可执行程序。例如，如果想针对 account 服务，可以用如下匹配模式：
# pkill -e -f "[g]o run app/account/rpc/account.go" || true
# pkill -e -f "/tmp/go-build.*exe/account" || true
# 注意：  
# 使用 "[g]o run ..." 可以防止误杀命令自身；  
# “/tmp/go-build.*exe/account” 这个正则匹配在命令行中出现的可执行文件路径，确保所有通过 go build 生成并运行的 account 服务进程也能被杀掉。

# 4、【方案2：使用 killall 或 pid 文件】
# 如果在生产或开发过程中允许，可以直接使用 killall 指令（前提是确定系统中不会有同名其他重要进程），例如：
# killall account
# killall admin
# …
# 或让各个服务将自己的 pid 保存到文件中，然后利用这些 pid 文件来发送 kill 命令。

# 5、总结：
# 出现“/tmp/go-build…”相关进程没有被杀，是因为当前的 pkill 匹配模式只匹配了 “go run app/…” 的字符串，而编译后的可执行文件不包含该字符串。需要更改匹配模式来覆盖这两种情况，以确保所有服务进程都能被正确停止。例如，可以在 Makefile 中添加对 “/tmp/go-build” 的匹配。

# make: *** [Makefile:...: stop-local] 已终止现象分析：
# 目前“已终止”的提示很可能是因为某个 pkill 命令匹配范围过宽或模式不够精确，导致不小心杀掉了当前 shell（或关联的辅助进程），从而导致整个命令块被终止。可以做如下调整：
# 1、精细调整正则表达式，确保只匹配目标服务的进程命令行；
# 精确匹配进程：修改 kill 命令的匹配模式，尽量只匹配目标程序的独特标识，而不要用太宽泛的正则表达式。
# 例如，可以使用固定的服务名称（如果可执行程序的名称足够独特）：
# pkill -e -f "/tmp/go-build.*exe/account\s" || true
# 或者使用其他更不容易误杀当前 Shell 的匹配规则。
# 2、可以为不同服务分别执行 pkill 命令，并在 Makefile 中对每条命令加上 “-” 前缀（忽略错误），防止单个命令的失败中断整个目标；
# 例如：-@sh -c 'pkill -e -f "/tmp/go-build.*exe/account" || true'
# 3、更稳妥的方案是使用 pid 文件来管理服务进程，再用 kill 对 pid 文件内容进行操作。
# 这样调整后，应该就不会出现提示“已终止”而进程仍然残留的情况。

# 查看进程：
# ps aux | grep "[g]o run app" | awk '{print $2}' 
# ps aux | grep "[g]o-build"
# 看看实际匹配到哪些进程，确认这些进程确实就是所希望停止的，而不是关键的 shell 或其他辅助进程。如果发现误匹配，就需要调整正则匹配模式，使之更准确。
# pgrep -fl "/tmp/go-build.*exe/account"
stop-local:
	@echo "Stopping all local services..."
	@echo "Stopping RPC services..."
	@sh -c 'pkill -e -f "[g]o run app/account/rpc/account.go" || true; \
			   pkill -e -f "[g]o run app/match/rpc/match.go" || true; \
	           pkill -e -f "[g]o run app/order/rpc/order.go" || true; \
	           pkill -e -f "[g]o run app/quotes/kline/rpc/kline.go" || true; exit 0'
	@echo "Stopping API services..."
	@sh -c 'pkill -e -f "[g]o run app/account/api/account.go" || true; \
	           pkill -e -f "[g]o run app/match/mq/match.go" || true; \
	           pkill -e -f "[g]o run app/admin/api/admin.go" || true; \
	           pkill -e -f "[g]o run app/order/api/order.go" || true; \
	           pkill -e -f "[g]o run app/quotes/api/quote.go" || true; exit 0'
	@echo "Stopping go-build services..."			   
	@sh -c 'pkill -e -f "/tmp/go-build.*exe/account\s" || true; \
			   pkill -e -f "/tmp/go-build.*exe/match\s" || true; \
	           pkill -e -f "/tmp/go-build.*exe/order\s" || true; \
	           pkill -e -f "/tmp/go-build.*exe/kline\s" || true; \
			   pkill -e -f "/tmp/go-build.*exe/admin\s" || true; \
	           pkill -e -f "/tmp/go-build.*exe/quote\s" || true; exit 0'
	@echo "All local services stopped successfully"

# make run-mode MODE=container
# make run-mode MODE=local
run-mode:
	chmod +x ./deploy/scripts/run_mode.sh
	./deploy/scripts/run_mode.sh $(MODE)	