# Go命令
```bash
#  查看依赖
go list -m github.com/zeromicro/go-zero

# 清理旧依赖
go clean -modcache

# 更新依赖
go mod tidy
go mod verify

# 查看当前go版本
go version

# 查看go.mod中的框架版本
grep go-zero go.mod

# 安装grpcurl
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
# sudo cp $GOPATH/bin/grpcurl /usr/local/bin/

```
# 源码
```ini
/root/go/pkg/mod/go.opentelemetry.io/otel@v1.19.0/propagation/trace_context_test.go
```
# 通过Makefile部署的完整流程
## 快捷启动项目
```bash
# 1. 编译项目（构建应用服务）
make build 

# 2. 启动项目（基础设施+应用服务 dep1+dep2）
make run 
```
## 停止运行项目所有容器
```bash
make stop
# docker compose -f deploy/dockerfiles/docker-compose.yaml down
# docker compose -f deploy/depend/docker-compose.yaml down	
```
## 基础设施容器的前端UI
```ini
# 1. pulsar_admin 主界面（推荐）
# 用户名：admin 密码：pulsar
# 登录成功后，新建环境变量
# Environment Name ： pulsar-standalone
# Service URL： http://192.168.1.4:8080 或 http://pulsar:8080
http://localhost:9527/

# 进入租户管理页面
http://localhost:9527/#/management/tenants

# 2. pulsar_manager 管理端口（仅用于健康检查，无UI）
http://localhost:7750/
http://localhost:7750/pulsar-manager/

# 3. Jaeger UI
http://localhost:16686/

# 传入TraceID查看链路日志
# 通过OpenTelemetry分布式追踪,将日志写入Jaeger
http://localhost:16686/trace/e3dddcb8db99b0d0aeafed2d1829af99
```
### pulsar_manager
**参考文档**
``` ini
# https://pulsar.apache.org/docs/next/administration-pulsar-manager/
```
**配置pulsar_manager**
主要修改`pulsar-manager`和`application.properties`, 其中`pulsar-manager`是启动脚本, `application.properties`是pulsar_manager的配置文件。
容器中的/pulsar-manager/pulsar-manager/bin/pulsar-manager脚本硬编码了类路径，导致新增的mysql驱动包无法加载，通过volumes挂载新的脚本来解决。
``` bash
# 查看容器内路径
docker exec pulsar ls -la /pulsar/conf

# 从容器复制文件到宿主机
## 从容器复制pulsar所有配置文件到宿主机
sudo docker cp pulsar:/pulsar/conf /git/gex/deploy/depend/pulsar/
## 从容器复制pulsar-manager配置文件到宿主机(修改后再通过volumes挂载到容器)
sudo docker cp pulsar_manager:/pulsar-manager/pulsar-manager/application.properties /git/gex/deploy/depend/pulsar_manager/
## 从容器复制pulsar-manager部署包到宿主机
sudo docker cp pulsar_manager:/pulsar-manager/pulsar-manager/lib/pulsar-manager.jar /git/gex/deploy/depend/pulsar_manager/lib/
## 从容器复制数据库初始化脚本到宿主机
sudo docker cp pulsar_manager:/pulsar-manager/init_db.sql /git/gex/deploy/depend/pulsar_manager/
## 从容器复制pulsar-manager启动脚本到宿主机(修改后再通过volumes挂载到容器)
sudo docker cp pulsar_manager:/pulsar-manager/pulsar-manager/bin/pulsar-manager /git/gex/deploy/depend/pulsar_manager/bin/pulsar-manager.image

# 查看容器中的配置文件
docker exec pulsar_manager cat /pulsar-manager/pulsar-manager/application.properties | grep mysql

# 检查容器中的MySQL数据库依赖包
docker exec pulsar_manager ls -la /pulsar-manager/pulsar-manager/lib | grep mysql

# 验证MySQL数据库连接
docker exec mysql8 mysql -upulsar -ppulsar -e "SHOW DATABASES;"
```

**重启pulsar_manager服务**
``` bash
docker compose -f deploy/depend/docker-compose.yaml down pulsar_manager
docker compose -f deploy/depend/docker-compose.yaml up -d --force-recreate pulsar_manager
docker compose -f deploy/depend/docker-compose.yaml up -d --build pulsar_manager
```

**查看pulsar_manager详细日志**
``` bash
docker exec pulsar_manager tail -f /pulsar-manager/pulsar-manager/pulsar-manager.log

docker exec pulsar_manager tail -n 200 /pulsar-manager/pulsar-manager/pulsar-manager.log
```
**查看pulsar_manager类路径**
``` bash
# 正确输出应显示包含mysql驱动路径的通配符模式
docker exec pulsar_manager sh -c 'echo $CLASSPATH'

# 验证驱动加载
docker exec pulsar_manager sh -c 'ls -l /pulsar-manager/pulsar-manager/lib/mysql-connector*'
docker logs pulsar_manager | grep -i 'success'
docker exec pulsar_manager tail -n 200 /pulsar-manager/pulsar-manager/pulsar-manager.log | grep -i 'Executing SQL'

# 显示包含完整classpath的启动命令
docker exec pulsar_manager ps aux | grep java
```
**Set the administrator account and password**
调用curl之前，需要先修改`users`表的`access_token`字段可为`null`，否则会返回`Column 'access_token' cannot be null`的错误。
``` bash
CSRF_TOKEN=$(curl http://localhost:7750/pulsar-manager/csrf-token)
echo $CSRF_TOKEN
curl \
   -H 'X-XSRF-TOKEN: $CSRF_TOKEN' \
   -H 'Cookie: XSRF-TOKEN=$CSRF_TOKEN;' \
   -H "Content-Type: application/json" \
   -X PUT http://localhost:7750/pulsar-manager/users/superuser \
   -d '{"name": "admin", "password": "pulsar", "description": "test", "email": "username@test.org"}'
```

**pulsar用户初始化脚本**
以上的curl命令如不可行，可用sql脚本。
以下步骤可参考思路，实际脚本见`/git/gex/deploy/db/mysql_pulsar_manager_init.sql`中的三张表的三条插入语句。
``` bash
# 生成密码哈希（示例密码Admin@1234）
docker run --rm httpd:2.4-alpine htpasswd -bnBC 10 "" Admin@1234 | tr -d ':\n'
# 输出示例：$2a$10$5VY1TL8mYvbg6sP6e5PvE.8H8z7W1oL9VcB1J6dZ7g6sKj1X1n5rO

# 插入用户（注意字段对应关系）
docker exec mysql8 mysql -upulsar -ppulsar pulsar_manager -e \
"INSERT INTO users (user_id, name, email, password) VALUES (
    1,
    'admin',
    'admin@gex.com', 
    '$2a$10$5VY1TL8mYvbg6sP6e5PvE.8H8z7W1oL9VcB1J6dZ7g6sKj1X1n5rO' 
);"
```

## 其他命令
``` bash
# 启动基础设施
make dep1

# 启动应用服务
make dep2

# 检查容器状态
docker ps 

# 删除所有的none镜像
docker images -a | grep none | awk '{print $3}' | xargs docker rmi

# 参数说明：
# -d：后台运行
# --service-ports：暴露服务端口
# --build：重新构建镜像
# --rm：退出后自动删除容器（适合调试场景）
# -e：设置单个环境变量（可重复使用）
# docker compose -f deploy/dockerfiles/docker-compose.yaml run --rm -e OTEL_LOG_LEVEL=debug accountrpc
docker compose -f deploy/dockerfiles/docker-compose.yaml run -d --service-ports --build --rm -e OTEL_LOG_LEVEL=debug accountrpc

# 进入容器查看环境变量
docker exec accountrpc env | grep OTEL

# 进入容器查看配置文件
docker exec accountrpc cat /app/account.yaml | grep -E 'otlp|endpoint|insecure|timeout|exportType'

# 检查导出器文件权限
docker exec accountrpc sh -c "ls -l /dev/stdout"

# 查看所有注册服务
docker exec etcd etcdctl get --prefix "" --keys-only

# 验证容器间通信（在accountrpc容器中执行）
docker exec accountrpc sh -c "nc -zv etcd 2379"
# 成功应显示：etcd (172.23.0.3:2379) open

# 手动注册测试（在accountrpc容器中执行）
docker exec accountrpc sh -c "echo -e 'put /testkey \"hello\"\nquit' | etcdctl --endpoints=etcd:2379"
docker exec etcd etcdctl get /testkey
# 应输出：/testkey hello

# 带解码参数的查看方式（显示完整二进制数据）
docker exec etcd etcdctl get --prefix accountRpc/ --print-value-only | hexdump -C

# 查看指定服务实例
docker exec etcd etcdctl get --prefix accountRpc
docker exec -it etcd etcdctl get language/zh-CN
docker exec -it etcd etcdctl get Coin/IKUN
docker exec -it etcd etcdctl get Coin/USDT
docker exec -it etcd etcdctl get Symbol/IKUN_USDT

# 查询所有 key（不需要前导斜杠）
docker exec etcd etcdctl get --prefix ""

# 按实际前缀查询
docker exec etcd etcdctl get --prefix Symbol
docker exec etcd etcdctl get --prefix dtm
docker exec etcd etcdctl get --prefix klineRpc
docker exec etcd etcdctl get --prefix matchRpc
docker exec etcd etcdctl get --prefix orderRpc
docker exec etcd etcdctl get --prefix orderRpc/IKUN_USDT
docker exec etcd etcdctl get --prefix proxy

docker exec etcd etcdctl get --prefix accountRpc
# 输出结果解读
accountRpc/112474521994867649  # Key格式：服务名/实例ID <service-name>/<instance-id>
172.23.0.14:20002              # Value格式：服务实例IP:Port

# 进入accountrpc容器测试服务连通性
docker exec accountrpc curl 172.23.0.14:20002/health

# 使用更精确的路径格式查询
docker exec etcd etcdctl get --prefix /xxxxx

# 查看accountrpc容器日志中的注册信息
docker logs accountrpc | grep -E 'register|etcd'

# 进入accountrpc容器测试etcd连通性
docker ps | grep accountrpc
docker exec -it accountrpc sh
telnet etcd 2379

# 检查etcd健康状态
docker exec etcd etcdctl endpoint health
# 正常应显示：127.0.0.1:2379 is healthy: successfully committed proposal: took = 1.475798ms

# 验证配置文件存在性
docker exec accountrpc ls -l /app/account.yaml

# 检查配置文件内容 应显示包含etcd配置的内容
docker exec accountrpc cat /app/account.yaml | grep -A 5 'RpcServerConf'

# 查看pulsar-manager配置文件
docker exec pulsar_manager cat /pulsar-manager/pulsar-manager/application.properties

# 在accountrpc容器内测试连接jaeger
docker exec accountrpc curl -s http://jaeger:14269/metrics

# 进入容器查看环境变量
docker exec jaeger env | grep OTLP

# 查看环境变量生效情况
docker inspect jaeger | grep -A 20 'Env'
docker exec -it jaeger /go/bin/all-in-one-linux env

# 检查Jaeger的参数兼容性
docker exec jaeger /go/bin/all-in-one-linux --help | grep "grpc-server.enable-reflection"
docker exec jaeger /go/bin/all-in-one-linux --help | grep -E 'reflection|enable-reflection'
# 正确输出应包含该参数说明

# jaeger 命令参数
docker exec -it jaeger /go/bin/all-in-one-linux help

```

## 修改后重新构建容器
```bash
##  一键重新构建accountrpc
make rebuild-accountrpc

## 1. 停止并删除旧容器
docker stop accountrpc && docker rm accountrpc

## 2. 编译最新二进制文件
# make build
go build -ldflags="-s -w" -o ./bin/accountrpc ./app/account/rpc/account.go

## 3. 重新构建指定服务的镜像
docker compose -f deploy/dockerfiles/docker-compose.yaml build accountrpc

## 4. 重新启动容器
docker compose -f deploy/dockerfiles/docker-compose.yaml up -d --force-recreate accountrpc
docker compose -f deploy/dockerfiles/docker-compose.yaml up -d --build accountrpc

## 5. 验证镜像是否更新
# 查看镜像更新时间
docker images | grep accountrpc

# 查看容器使用的镜像ID
docker inspect accountrpc | grep Image
```
## 强制重新注册方法
```bash
# 1. 重启accountrpc服务
docker restart accountrpc

# 2. 观察注册日志（关键指标）
docker logs accountrpc | grep -iE 'register|etcd' -A 5 -B 5
docker logs accountrpc -f | grep -iE 'register|etcd|RpcServerConf|successfully'
docker logs accountrpc | grep 'RpcServerConf.Etcd'

# 3. 查看请求处理日志
docker logs accountrpc | grep 'RPC请求开始处理'

# 4. 再次检查注册信息
docker exec etcd etcdctl get --prefix /accountRpc/ --keys-only
docker exec etcd etcdctl get --prefix / --keys-only
```

## pulsar启动失败修复流程
```bash
# 1. 清理环境
docker compose -f deploy/depend/docker-compose.yaml down pulsar
# 停止并删除Pulsar容器
docker stop pulsar && docker rm pulsar
# 清理持久化数据
sudo rm -rf deploy/depend/pulsar/data/*

# 2. 重新启动
## 先确保pulsar完全启动
docker compose -f deploy/depend/docker-compose.yaml up -d pulsar
docker logs pulsar -f  # 观察直到显示"Successfully initialize managed ledger"

## 再启动其他服务
docker compose -f deploy/depend/docker-compose.yaml up -d

# 3. 等待BookKeeper初始化完成（至少2分钟）
watch -n 5 'docker logs pulsar | tail -n 20'

# 4. 执行完整部署流程
./deploy/scripts/run.sh

# 5. 检查Pulsar状态
docker logs pulsar | grep "Pulsar is running"
curl -f pulsar:8080/admin/v2/clusters
curl -f pulsar:8080/admin/v2/tenants
curl -f pulsar:8080/admin/v2/namespaces/public
curl -f pulsar:8080/admin/v2/namespaces/pulsar
curl -f pulsar:8080/admin/v2/namespaces/pulsar/system
curl -f pulsar:8080/admin/v2/namespaces/public/default
curl -f pulsar:8080/admin/v2/brokers/health

curl -f http://192.168.1.4:8080/admin/v2/namespaces/pulsar/system

curl -f http://localhost:8080/admin/v2/clusters
curl -f http://localhost:8080/admin/v2/brokers/health
```

## 创建测试账号
``` bash
# 编译并执行注册脚本
# 将生成的用户账号和密码复制到users_xxxx.txt文件，trade库中的user表和asset表中会有用户账号和资产信息
go build -o bin/register app/scripts/register.go
./bin/register
```

## 创建测试订单
``` bash
# 编译并执行注册脚本
go build -o bin/order app/scripts/order.go
./bin/order  -f app/account/api/etc/account_local.yaml

go run app/scripts/order.go -f app/account/api/etc/account_local.yaml 
```

## 访问接口验证
### 登录接口
```bash
# 登录请求

# 指定本地服务器IP和端口
curl -X POST http://localhost:20024/account/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}'

# 不指定端口，默认使用Nginx的默认端口80
curl -X POST http://localhost/account/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}'


# Save response to file 
# Verbose output
# Single-line version (recommended):
curl -X POST http://localhost/account/v1/login -H "Content-Type: application/json" -d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}' -o response.json -v

# Multi-line version (if you prefer readability):
curl -X POST http://localhost/account/v1/login \
-H "Content-Type: application/json" \
-d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}' \
-o response.json \
-v

 # Save cookies
 # Follow redirects
 # Single-line version (recommended):
curl -X POST http://localhost/account/v1/login -H "Content-Type: application/json" -d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}' -c cookies.txt -L
# Multi-line version (if you prefer readability):
curl -X POST http://localhost/account/v1/login \
-H "Content-Type: application/json" \
-d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}' \
-c cookies.txt \
-L                  
```

### 市价买单接口
```bash
# 一般思路(需通过命令登录获取token)
# 1. 先获取登录token
LOGIN_RESP=$(curl -s -X POST http://api.gex.com/account/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}')
echo $LOGIN_RESP

# 2. 提取token（使用jq工具）
TOKEN=$(echo $LOGIN_RESP | jq -r '.data.token')
USER_ID=$(echo $LOGIN_RESP | jq -r '.data.uid')

# 3. 发送创建订单请求（携带token）
curl -X POST http://api.gex.com/order/v1/create_order \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "symbol_name": "IKUN_USDT",
    "side": 1, 
    "order_type": 2,
    "amount": "100"
  }'  

# 直接通过浏览器登录后手动复制token
curl -X POST http://api.gex.com/order/v1/create_order \
  -H "Accept: application/json, text/plain, */*" \
  -H "Content-Type: application/json;charset=UTF-8" \
  -H "gexToken: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySUQiOjExMiwiVXNlcm5hbWUiOiJ0ZXN0MSIsIk5pY2tOYW1lIjoiIiwiaXNzIjoiemhhbmdzYW4iLCJhdWQiOlsiR1ZBIl0sImV4cCI6MTc0ODc0MTMwNCwibmJmIjoxNzQ3ODc3MzA0fQ.y6XhWi4BfBKV5NcJuy3WBo8Xb8Xe4BnpRxQvvjWLtcg" \
  -H "gexUserId: 112" \
  -d '{
    "symbol_name": "IKUN_USDT",
    "side": 1,
    "order_type": 2,
    "price": "0.03",
    "qty": "10",
    "amount": ""
  }'  
```
### 获取订单列表
```bash
# 建议优先使用单行命令，除非命令特别长需要换行以提高可读性
# Single-line version (recommended):
curl -X POST http://api.gex.com/order/v1/get_order_list -H "Content-Type: application/json;charset=UTF-8" -H "gexToken: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySUQiOjExMiwiVXNlcm5hbWUiOiJ0ZXN0MSIsIk5pY2tOYW1lIjoiIiwiaXNzIjoiemhhbmdzYW4iLCJhdWQiOlsiR1ZBIl0sImV4cCI6MTc0ODc0MTMwNCwibmJmIjoxNzQ3ODc3MzA0fQ.y6XhWi4BfBKV5NcJuy3WBo8Xb8Xe4BnpRxQvvjWLtcg" -H "gexUserId: 112" -d '{"status_list":["1","2"],"symbol_name":"IKUN_USDT","page_size":10,"id":"0"}'

# \后面不能有空格或其他字符
# Multi-line version (if you prefer readability):
curl -X POST http://api.gex.com/order/v1/get_order_list \
  -H "Content-Type: application/json;charset=UTF-8" \
  -H "gexToken: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJVc2VySUQiOjExMiwiVXNlcm5hbWUiOiJ0ZXN0MSIsIk5pY2tOYW1lIjoiIiwiaXNzIjoiemhhbmdzYW4iLCJhdWQiOlsiR1ZBIl0sImV4cCI6MTc0ODc0MTMwNCwibmJmIjoxNzQ3ODc3MzA0fQ.y6XhWi4BfBKV5NcJuy3WBo8Xb8Xe4BnpRxQvvjWLtcg" \
  -H "gexUserId: 112" \
  -d '{"status_list":["1","2"],"symbol_name":"IKUN_USDT","page_size":10,"id":"0"}'
#  
```


# 前端环境部署
## 构建前端
``` bash
# 安装依赖
yarn install
# 构建前端
yarn build
```
将build生成的dist目录拷贝到项目的`deploy/depend/nginx/html`目录下

## 访问前端
### 配置一个host 映射
```
192.168.1.4        api.gex.com
```
### 访问浏览器
```
http://api.gex.com/

预设账号：
用户名: lisi
密码: lisilisi
该账号已预分配10,000 USDT和1,000 IKUN
```

# 后端环境部署
## 启动基础设施
通过 Docker 容器启动etcd、mysql、redis、pulsar等基础设施
``` bash
# 创建外部网络
# 容器内网络：容器间通过Docker DNS自动解析容器名称
docker network create gex

# 构建ws_socket镜像
chmod +x deploy/depend/ws/socket/socket
docker compose -f deploy/depend/docker-compose.yaml build wssocket

# 构建ws_proxy镜像
chmod +x deploy/depend/ws/proxy/proxy
docker compose -f deploy/depend/docker-compose.yaml build wsproxy

# 启动基础设施
docker compose -f deploy/depend/docker-compose.yaml down
docker compose -f deploy/depend/docker-compose.yaml up -d

# 跟踪启动容器
docker compose -f deploy/depend/docker-compose.yaml up nginx
docker compose -f deploy/depend/docker-compose.yaml restart nginx

# 单独启动容器
docker compose -f deploy/depend/docker-compose.yaml up -d adminapi

# 进入容器
docker exec -it pulsar bash

# 验证服务互通
docker exec -it nginx ping adminapi

# 测试容器间通信（在容器内执行）
docker exec accountrpc ping etcd  # 应能解析容器IP

# 验证所有服务是否都在同一个Docker网络（gex）中
docker network inspect gex

# 检查jaeger服务是否在gex网络中
docker network inspect gex | grep jaeger 

# 检查某个容器是否正在运行
docker ps | grep adminapi

# 检查容器日志
docker logs nginx
docker logs pulsar -f -n 100

# 检查某个容器
docker inspect pulsar
```

## 部署后端服务
``` bash
# 编译项目
make build

# 启动后端服务
docker compose -f deploy/dockerfiles/docker-compose.yaml up -d
```

# 开发模式运行微服务
## 方案一：配置调整-IP替换容器名称
### 1.获取Docker网络网关IP
宿主机访问：需要通过网关IP或端口映射访问容器服务。
宿主机访问容器：
方法一：使用172.23.0.1 + 容器内部端口
方法二：使用127.0.0.1 + 映射到宿主机的端口（需要配置docker-compose端口映射）
```bash
# 一般输出结果为：172.23.0.1
docker network inspect gex --format='{{range .IPAM.Config}}{{.Gateway}}{{end}}'
# 一般输出结果为："Gateway": "172.23.0.1"
docker network inspect gex | grep Gateway
```
### 2.修改本地服务配置
`account_local.yaml`
```yaml
AccountRpcConf:
  etcd:
    Key: accountRpc
    Hosts:
      - 172.23.0.1:2379  # 使用网关IP代替容器名称
```
### 3.验证网络连通性
```bash
# 测试etcd连接
telnet 172.23.0.1 2379

# 测试pulsar连接
telnet 172.23.0.1 8080

# 测试redis连接 输入quit退出
telnet 172.23.0.1 6379

# 测试mysql连接  输入exit或quit退出
mysql -h 172.23.0.1 -P 3307 -uroot -proot
```
## 方案二：配置调整-修改hosts文件（推荐）
这种方案的优点是：
1.保持配置文件与生产环境完全一致
2.通过hosts映射实现本地调试
3.端口映射清晰可见，便于管理
注意事项：
1.确保Docker容器实际监听了这些端口
2.不同服务不要使用相同宿主机端口
3.如果修改了容器名称需要同步更新hosts文件
### 1.添加以下内容到/etc/hosts
修改Windows hosts文件即可同步到WSL.
```bash
192.168.1.4        api.gex.com
# Docker网络网关IP
172.23.0.1         pulsar pulsar_manager mysql8 redis etcd ws_proxy ws_socket dtm jaeger
```
由于使用的是WSL,hosts文件在WSL启动时自动生成，里面的内容与Windows hosts文件一致。
```bash
root@LenovoLG:/git/gex# cat /etc/hosts
# This file was automatically generated by WSL. To stop automatic generation of this file, add the following entry to /etc/wsl.conf:
# [network]
# generateHosts = false
```
### 2.验证网络连通性
```bash
# 测试etcd连接
curl http://etcd:2379/version
curl http://etcd:2379/health

# 测试mysql连接
mysql -h mysql8 -P 3307 -uroot -proot

# 测试redis连接
redis-cli -h redis -p 6379
telnet redis 6379

# 测试宿主机访问
ping etcd          # 应指向172.23.0.1
telnet etcd 2379   # 验证端口连通性

# 测试jaeger连接
docker exec -it accountrpc sh
telnet jaeger 4317
docker exec jaeger sh -c "netstat -tuln | grep 4317"

# Jaeger 的 OTLP HTTP 接收器地址
curl -v http://jaeger:4318/v1/traces

# 使用grpcurl检查jaeger服务列表
docker exec jaeger env | grep GRPC
grpcurl -plaintext jaeger:4317 list
# Failed to list services: server does not support the reflection API

# 使用grpcurl检查jaeger服务健康状态
grpcurl -plaintext -d '{}' jaeger:4317 grpc.health.v1.Health/Check

# 使用 HTTP 端点验证 Jaeger 是否正常工作
curl http://jaeger:14269/metrics | grep 'jaeger_'

# 检查 collector 状态 使用 HTTP 端点验证服务状态
curl http://localhost:16686/api/services | jq
curl http://jaeger:16686/api/services | jq
curl -s http://jaeger:16686/api/services | jq

# 检查 metrics 接口
curl -s http://jaeger:16686/metrics | grep 'grpc_server'

# 查看暴露的 gRPC 服务列表（需提前获取 proto 文件）
grpcurl -plaintext -proto jaeger/proto/api_v2/query.proto jaeger:16685 list

# grpcurl直接调用 proto 文件
## 下载 jaeger 的 proto 文件
git clone https://github.com/jaegertracing/jaeger.git --depth=1

## 使用本地 proto 文件执行调用
grpcurl -plaintext -proto jaeger/proto/api_v2/query.proto -proto jaeger/proto/api_v2/model.proto jaeger:4317 jaeger.api_v2.QueryService/GetServices

```
## 检查容器服务运行状态
关键错误排查点：
1.etcd服务是否可用
2.account.rpc服务是否注册
3.网络端口映射是否正确
4.本地服务配置是否指向有效etcd地址
```bash
# 查看etcd中的注册信息（应有accountRpc的键值）
docker exec etcd etcdctl get --prefix accountRpc
# 可停止容器后只启动本地服务便于开发调试，如果有多个服务注册时输出如下：
accountRpc/112474613211408096
172.23.0.17:20002
accountRpc/112474613211408760
192.168.1.4:20012

# 查看accountrpc容器状态
docker ps | grep accountrpc

# 查看服务注册日志
docker logs accountrpc | grep 'register service'

# 重启account.rpc容器
docker restart accountrpc

# 查看注册日志（关键指标）
docker logs accountrpc | grep -E 'register service|etcd'
```

## 停止开发的docker容器（可选）
```bash
docker compose -f deploy/dockerfiles/docker-compose.yaml down accountapi
```
##  启动本地开发服务
```bash
# 拷贝自account.yaml文件，修改端口避免冲突
go run app/account/api/account.go -f app/account/api/etc/account_local.yaml

netstat -tuln | grep 20014  
netstat -tuln | grep 20024  
```

## 测试本地开发服务接口
```bash
# 指定本地服务器IP和端口
curl -X POST http://localhost:20024/account/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"lisi","password":"lisilisi","captcha":"885778","captcha_id":"BsDmxQSZoghXeyaybOD8"}'
 ``` 


 # Go-Zero-Micro
 ## API层和RPC层
 在go-zero的架构中，通常会将服务分为API层和RPC层。API层处理HTTP请求，通过RPC调用后端的业务逻辑服务。这里的accountapi作为API层，接收HTTP请求，然后通过etcd发现accountrpc服务，发起gRPC调用。这种分层符合go-zero的推荐结构，API负责协议转换和路由，RPC负责业务逻辑。
 1. **服务架构**

 关系图：
```plaintext
Nginx(80) → accountapi(20014) → accountrpc(20002)
              (HTTP)                 (gRPC)
```
示意图：
```text
              [HTTP请求]
                  │
                  ▼
           ┌───────────────┐
           │   accountapi  │
           │  (HTTP服务)   │
           └───────────────┘
                  │ 通过etcd发现
                  ▼
           ┌───────────────┐
           │   accountrpc  │
           │  (gRPC服务)   │◄──注册到etcd
           └───────────────┘
```
2. **服务职责**

| 服务 | 类型 | 端口 | 协议 | 职责 | 代码示例 | 

|---------------|--------|---------|--------|-------------------------------|---------------------------| 

| accountapi | API | 20014 | HTTP | 对外暴露RESTful接口，参数校验 | app/account/api/internal/handler |

| accountrpc | RPC | 20002 | gRPC | 核心业务逻辑，数据持久化 | app/account/rpc/internal/logic |

3. **go-zero规范解读**

3.1.分层架构：符合go-zero推荐的API层+RPC层分离模式
```text
API层：处理HTTP协议转换、参数校验、鉴权等
RPC层：专注业务逻辑实现，通过protobuf定义服务
```
3.2.服务发现：通过etcd实现服务注册发现
`app/account/api/internal/svc/service_context.go`
```go
func NewServiceContext(c config.Config) *ServiceContext {
	sc := &ServiceContext{
		Config:           c,
		Auth:             middleware.NewAuthMiddleware(accountservice.NewAccountService(zrpc.MustNewClient(c.AccountRpcConf))).Handle,
		AccountRpcClient: accountservice.NewAccountService(zrpc.MustNewClient(c.AccountRpcConf)), // 通过etcd发现服务
		CaptchaStore:     base64Captcha.NewMemoryStore(10000, time.Minute*3),
	}

	return sc
}
```
3.3.端口规范：
- API服务使用2xxxx端口段（如20014）
- RPC服务使用2xxxx端口段（如20002）
- 通过Docker网络进行服务间通信

4. **调用流程示例**
```text
用户请求 → Nginx(80) → accountapi(20014) → etcd查询服务地址 → accountrpc(20002)
```
5. **服务调用流程**
```plaintext
HTTP请求 → Nginx → accountapi → etcd → accountrpc
```
6. **服务调用时序图**
```mermaid
sequenceDiagram
    participant HTTP as HTTP请求
    participant Nginx as Nginx
    participant accountapi as accountapi
    participant etcd as etcd
    participant accountrpc as accountrpc
    HTTP->>Nginx: 发送HTTP请求
    Nginx->>accountapi: 转发请求
    accountapi->>etcd: 发现accountrpc服务
    etcd-->>accountapi: 返回accountrpc服务地址
    accountapi->>accountrpc: 发起gRPC调用
    accountrpc-->>accountapi: 返回结果
    accountapi-->>Nginx: 返回HTTP响应
    Nginx-->>HTTP: 返回HTTP响应
```
7. **服务注册流程**
```plaintext
accountrpc启动时 → etcd注册服务 → accountapi发现服务
```
8. **服务注册时序图**
```mermaid
sequenceDiagram
    participant accountrpc as accountrpc
    participant etcd as etcd
    accountrpc->>etcd: 注册服务
    etcd-->>accountrpc: 返回注册结果
```
9. **服务发现流程**
```plaintext
accountapi通过etcd发现accountrpc服务 → 发起gRPC调用
```
10. **服务发现时序图**
```mermaid
sequenceDiagram
    participant accountapi as accountapi
    participant etcd as etcd
    accountapi->>etcd: 发现accountrpc服务
    etcd-->>accountapi: 返回accountrpc服务地址
```