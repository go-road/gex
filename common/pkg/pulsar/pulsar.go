package pulsar

import (
	"strings"
	"time"

	"github.com/apache/pulsar-client-go/pulsar"
	pulsarLog "github.com/apache/pulsar-client-go/pulsar/log"
	"github.com/dtm-labs/logger"
	"github.com/sirupsen/logrus"
)

type PulsarConfig struct {
	Hosts []string `json:"hosts" yaml:"Hosts"`
}

func (pc PulsarConfig) BuildClient() (pulsar.Client, error) {
	logger := logrus.StandardLogger()
	logger.Level = logrus.WarnLevel
	addr := make([]string, 0, len(pc.Hosts))
	for _, v := range pc.Hosts {
		addr = append(addr, "pulsar://"+v)
	}
	url := strings.Join(addr, ",")

	logger.Infof("connect to pulsar %s", url)
	// 增加pulsar连接重试机制
    var pulsarClient pulsar.Client
    for i := 0; i < 5; i++ {
		client, err := pulsar.NewClient(pulsar.ClientOptions{
			URL:               url,
			OperationTimeout:  30 * time.Second,
			ConnectionTimeout: 30 * time.Second,
			Logger:            pulsarLog.NewLoggerWithLogrus(logger),
		})
		// if err != nil {
		// 	return nil, err
		// }

        if err == nil {
            pulsarClient = client
            break
        }
        time.Sleep(2 * time.Second)
    }
    if pulsarClient == nil {
		logger.Panicf("init pulsar consumer failed after retries %s", url)
    }


	return pulsarClient, nil
}

type Topic struct {
	Tenant    string
	Namespace string
	Topic     string
}

// Topic主要用于传递撮合引擎的撮合结果到各个下游服务
func (t Topic) BuildTopic() string {
	topic := "persistent://" + t.Tenant + "/" + t.Namespace + "/" + t.Topic
	logger.Infof("build pulsar topic %s", topic)
	return topic
}

const (
	PublicTenant          = "public"
	GexNamespace          = "trade"
	MatchSourceTopic      = "match_source"
	MatchResultTopic      = "match_result"
	MatchResultAccountSub = "MatchResultAccountSub"
	MatchSourceSub        = "match_source_sub"
	MatchResultOrderSub   = "MatchResultOrderSub"
	MatchResultKlineSub   = "MatchResultKlineSub"
	MatchResultTickerSub  = "MatchResultTickerSub"
	MatchResultMatchSub   = "MatchResultMatchSub"
)
