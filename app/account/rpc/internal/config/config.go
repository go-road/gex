package config

import (
	"github.com/luxun9527/gex/common/pkg/etcd"
	commongorm "github.com/luxun9527/gex/common/pkg/gorm"
	"github.com/luxun9527/gex/common/pkg/pulsar"
	logger "github.com/luxun9527/zlog"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	GormConf         commongorm.GormConf
	LoggerConfig     logger.Config
	PulsarConfig     pulsar.PulsarConfig
	RedisConf        redis.RedisConf
	SymbolEtcdConfig etcd.EtcdConfig
	OTLP             OTLPConfig `yaml:"otlp" json:"otlp"`
}

type OTLPConfig struct {
	Endpoint   string `yaml:"endpoint" json:"endpoint"`     // 对应环境变量 OTEL_EXPORTER_OTLP_ENDPOINT
	Insecure   bool   `yaml:"insecure" json:"insecure"`     // 对应环境变量 OTEL_EXPORTER_OTLP_INSECURE
	Timeout    int    `yaml:"timeout" json:"timeout"`       // 单位秒，对应 OTEL_EXPORTER_OTLP_TIMEOUT
	ExportType string `yaml:"exportType" json:"exportType"` // grpc/http 对应 OTEL_EXPORTER_OTLP_PROTOCOL
}