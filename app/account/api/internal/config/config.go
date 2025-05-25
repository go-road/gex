package config

import (
	"github.com/luxun9527/gex/common/pkg/etcd"
	logger "github.com/luxun9527/zlog"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	Env string
	rest.RestConf
	LoggerConfig     logger.Config
	AccountRpcConf   zrpc.RpcClientConf
	LanguageEtcdConf etcd.EtcdConfig
}
