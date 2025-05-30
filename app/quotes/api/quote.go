package main

import (
	"flag"
	"github.com/luxun9527/gex/app/quotes/api/internal/config"
	"github.com/luxun9527/gex/app/quotes/api/internal/handler"
	"github.com/luxun9527/gex/app/quotes/api/internal/svc"
	logger "github.com/luxun9527/zlog"
	"github.com/zeromicro/go-zero/core/logx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "app/quotes/api/etc/quote.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	logx.DisableStat()
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	logx.SetLevel(logx.DebugLevel)
	logx.SetWriter(logger.NewZapWriter(logger.GetZapLogger()))
	logx.Infof("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
