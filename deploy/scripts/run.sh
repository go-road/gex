#!/bin/bash


# 检查名为"gex"的网络是否存在
network_exists=$(docker network ls --format "{{.Name}}" --filter "name=gex")
# 如果网络不存在，则创建名为"gex"的网络
if [ -z "$network_exists" ]; then
    docker network create gex
    echo "网络 gex 创建成功！"
fi

lang='50006: 超过最小精度
100001: 内部错误
100002: 内部错误
100003: 内部错误
100004: 参数错误
100005: 记录未找到
100006: 重复数据
100007: 内部错误
100009: 内部错误
100010: 内部错误
100011: 内部错误
100012: 验证码错误
200001: 用户不存在
200002: 用户余额不足
200003: token验证失败
200004: token到期
200005: 账户密码验证失败1
500001: 订单未找到
500002: 订单已经成交获取已经取消
500003: 市价单不允许手动取消
500004: 订单簿没有买单
500005: 订单簿没有卖单
500006: 超过币种最小精度'

coin1='coinid: 10001
coinname: IKUN
prec: 3'

coin2='coinid: 10002
coinname: USDT
prec: 5'
symbol='symbolname: IKUN_USDT
symbolid: 1
basecoinname: IKUN
basecoinid: 10001
quotecoinname: USDT
quotecoinid: 10002
baseCoinPrec: 3
quoteCoinPrec: 5'

# echo "Processing Pulsar file directory in the first launch..."
# mkdir -p deploy/depend/pulsar/data/metadata
# mkdir -p deploy/depend/pulsar/conf
# sudo rm -rf deploy/depend/pulsar/data/*
# sudo chmod -R a+rwx deploy/depend/pulsar/data
# sudo chmod 777 deploy/depend/pulsar/data
# sudo chmod 777 deploy/depend/pulsar/data/metadata
# sudo chmod -R 755 deploy/depend/pulsar/conf

echo "启动基础设施容器..."
docker compose -f deploy/depend/docker-compose.yaml up -d

sleep 30s

echo "初始化etcd配置..."
docker exec -it etcd /usr/local/bin/etcdctl put language/zh-CN -- "$lang"
docker exec -it etcd /usr/local/bin/etcdctl put Coin/IKUN -- "$coin1"
docker exec -it etcd /usr/local/bin/etcdctl put Coin/USDT -- "$coin2"
docker exec -it etcd /usr/local/bin/etcdctl put Symbol/IKUN_USDT -- "$symbol"

# The key is to ensure Pulsar is fully initialized before attempting to create topics. The health check and wait script will prevent these errors from occurring during startup.
# Wait for Pulsar to be ready
while ! curl -s http://localhost:8080/admin/v2/brokers/health > /dev/null; do
  echo "Waiting for Pulsar to start..."
  sleep 5
done

echo "创建Pulsar命名空间和主题..."
# docker exec -it pulsar /pulsar/bin/pulsar-admin namespaces create public/trade || true
# docker exec -it pulsar /pulsar/bin/pulsar-admin topics create persistent://public/trade/match_source_IKUN_USDT
# docker exec -it pulsar /pulsar/bin/pulsar-admin topics create persistent://public/trade/match_result_IKUN_USDT

echo "创建Pulsar命名空间..."
# 先检查租户是否存在
if ! docker exec pulsar /pulsar/bin/pulsar-admin tenants list | grep -q "public"; then
    docker exec pulsar /pulsar/bin/pulsar-admin tenants create public
    echo "租户 public 创建成功"
fi

# 检查命名空间时指定租户参数
if ! docker exec pulsar /pulsar/bin/pulsar-admin namespaces list public | grep -wq "trade"; then
    docker exec pulsar /pulsar/bin/pulsar-admin namespaces create public/trade
    echo "命名空间 public/trade 创建成功"
else
    echo "命名空间 public/trade 已存在，跳过创建"
fi

# Then create topics
# 主题创建部分为带存在性检查的重试机制
create_topic_if_not_exists() {
    local topic=$1
    if ! docker exec pulsar /pulsar/bin/pulsar-admin topics list public/trade | grep -q "^persistent://public/trade/$topic$"; then
        echo "正在创建主题 $topic..."
        docker exec pulsar /pulsar/bin/pulsar-admin topics create "persistent://public/trade/$topic"
        return $?
    else
        echo "主题 $topic 已存在，跳过创建"
        return 0
    fi
}

# 带重试的主题创建函数
create_topic_with_retry() {
    local topic=$1
    local max_retries=5
    local retry_count=0
    
    until create_topic_if_not_exists "$topic" || [ $retry_count -eq $max_retries ]; do
        echo "创建主题 $topic 失败，正在重试... ($((retry_count+1))/$max_retries)"
        sleep 10
        ((retry_count++))
    done
    
    if [ $retry_count -eq $max_retries ]; then
        echo "创建主题 $topic 达到最大重试次数，请手动检查！"
        return 1
    fi
}

# 调用创建函数
echo "创建Pulsar主题..."
create_topic_with_retry "match_source_IKUN_USDT"
create_topic_with_retry "match_result_IKUN_USDT"

# 容器首次启动成功后从容器拷贝pulsar配置文件
# docker cp pulsar:/pulsar/conf/. deploy/depend/pulsar/conf/

echo "启动应用服务容器..."
docker compose -f deploy/dockerfiles/docker-compose.yaml up -d



