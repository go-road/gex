package main

import (
	"context"
	"flag"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/trace"

	// "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"

	"github.com/luxun9527/gex/app/account/rpc/internal/config"
	"github.com/luxun9527/gex/app/account/rpc/internal/consumer"
	"github.com/luxun9527/gex/app/account/rpc/internal/server"
	"github.com/luxun9527/gex/app/account/rpc/internal/svc"
	"github.com/luxun9527/gex/app/account/rpc/pb"
	logger "github.com/luxun9527/zlog"
	"github.com/zeromicro/go-zero/core/logx"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "app/account/rpc/etc/account.yaml", "the config file")

// 日志级别过滤：添加OpenTelemetry调试日志
func init() {
    // 开启详细日志
    otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
        logx.Error("OpenTelemetry错误", logx.Field("error", err))
    }))
}

// 初始化追踪器
func initTracer() (*sdktrace.TracerProvider, error) {
	logx.Info("正在初始化Jaeger连接...")
    
    // 创建带超时的context
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    // 添加预连接检查
    conn, err := grpc.DialContext(ctx, "jaeger:4317", 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // 阻塞直到连接成功
	)
    if err != nil {
        return nil, fmt.Errorf("无法连接Jaeger: %v", err)
    }
    defer conn.Close()
	logx.Info("Jaeger连接成功,正在创建导出器...")

	// 创建OTLP HTTP exporter
	// exporter, err := otlptracehttp.New(context.Background(),
	// 	otlptracehttp.WithEndpoint("jaeger:4318"), // 使用容器名称访问
	// 	otlptracehttp.WithInsecure(),             // 非加密连接
	// 	otlptracehttp.WithURLPath(""),            // 清空路径
	// 	otlptracehttp.WithCompression(otlptracehttp.NoCompression), // 关闭压缩
	// 	otlptracehttp.WithHeaders(map[string]string{
    //         "Content-Type": "application/x-protobuf",
    //     }),
	// )

	// 创建OTLP gRPC exporter
	exporter, err := otlptracegrpc.New(ctx,
        otlptracegrpc.WithEndpoint("jaeger:4317"),
        otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithTimeout(5*time.Second),
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: 1 * time.Second,
			MaxInterval:     5 * time.Second,
			MaxElapsedTime:  30 * time.Second,
		}),
    )
	if err != nil {
		// logx.Error("创建导出器失败", logx.Field("error", err))
		logx.Errorf("创建导出器失败: %v", err)
		return nil, fmt.Errorf("创建导出器失败: %v", err) 
	}

	// 添加导出器健康检查
	// if err := exporter.Start(context.Background()); err != nil {
	// 	logx.Error("导出器启动失败", logx.Field("error", err))
	// 	return nil, err
	// }

	logx.Info("导出器创建成功,正在配置资源信息...")
	// 配置资源信息
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			// semconv.ServiceName("account.rpc"),
			semconv.ServiceNameKey.String("account.rpc"), // 强制指定服务名
			semconv.ServiceVersion("v1.0.0"),
			// semconv.DeploymentEnvironment("prod"),
			attribute.String("environment", "prod"),
			attribute.String("host.name", "account-rpc"),
            // attribute.String("service.version", "v1.2.3"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("配置资源信息失败: %v", err)
	}

	// 创建采样器
    // sampler := sdktrace.ParentBased(
    //     sdktrace.TraceIDRatioBased(0.5), // 50% 采样率
    // )
	sampler := sdktrace.AlwaysSample() //全部采样

	logx.Info("资源信息配置成功,正在创建TracerProvider...")
	// 创建TracerProvider
	tp := sdktrace.NewTracerProvider(
		// sdktrace.WithBatcher(exporter, 
        //     sdktrace.WithBatchTimeout(5*time.Second),
		// 	sdktrace.WithMaxExportBatchSize(512),
        //     sdktrace.WithMaxQueueSize(2048),
        //     sdktrace.WithExportTimeout(10*time.Second),
        // ),
		sdktrace.WithSyncer(exporter), // 同步导出
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)	
	logx.Info("TracerProvider创建成功")
	// 注册自定义处理器用于调试
	logx.Info("注册自定义span处理器")
	// tp.RegisterSpanProcessor(&logSpanProcessor{})
	// tp.RegisterSpanProcessor(NewLogSpanProcessor())
	processor := NewLogSpanProcessor()
	if processor == nil {
		return tp, fmt.Errorf("failed to create span processor")
	}
	tp.RegisterSpanProcessor(processor)
	// 设置全局TracerProvider
	otel.SetTracerProvider(tp)

	// 配置传播器
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, 
		propagation.Baggage{},
	))
	logx.Info("TracerProvider初始化完成")
	return tp, nil
}

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	ctx := svc.NewServiceContext(c)
	consumer.InitConsumer(ctx)

	// 添加注册验证日志
	logx.Infof("ETCD registration config: %+v", c.RpcServerConf.Etcd)
    logx.Infof("RpcServerConf.Etcd.Hosts = %v", c.RpcServerConf.Etcd.Hosts)
    logx.Infof("RpcServerConf.Etcd.Key = %s", c.RpcServerConf.Etcd.Key)

	// 初始化追踪器
	tp, err := initTracer()
	if err != nil {
		logx.Error("初始化追踪器失败", logx.Field("error", err))
		return
	}
	logx.Info("OpenTelemetry追踪器已初始化", 
    logx.Field("service", "account.rpc"),
    logx.Field("endpoint", "jaeger:4318"))
	defer tp.Shutdown(context.Background())

	// 自动注册到etcd
	// 框架会自动执行以下操作：
	// 1. 创建etcd客户端连接
	// 2. 生成lease
	// 3. 注册服务键值（格式：/accountRpc/xxxxxxxx）
	// 使用go-zero框架创建gRPC服务器
	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		logx.Infof("RpcServerConf: %+v", c.RpcServerConf)
        // 添加OpenTelemetry配置
		// 使用新版统计处理器（替换旧拦截器）
        grpc.StatsHandler(otelgrpc.NewServerHandler())  
        // grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor())

		// 注册protobuf定义的gRPC服务实现
		pb.RegisterAccountServiceServer(grpcServer, server.NewAccountServiceServer(ctx))
		logx.Infof("c.Mode is %+v , service.DevMode is %+v, service.TestMode is %+v", c.Mode, service.DevMode, service.TestMode)
		// 开发/测试模式启用gRPC反射服务（用于grpcurl调试）
		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
			logx.Infof("grpcServer registered successfully")
		}
	})

	// s.AddUnaryInterceptors(
	// 	tracing.ServerInterceptor(), // OpenTelemetry 拦截器
	// 	statinterceptor.NewStatInterceptor().Handle, // go-zero 统计拦截器
	// )

	s.AddOptions(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionAge: time.Minute * 30,
		}),
		// 添加注册回调日志
		// grpc.WithUnaryServerInterceptor(grpcx.Server.UnaryValidate),
		// logx.Info("Service registration completed")
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		    // 添加请求拦截器
			logx.Info("RPC请求开始处理", logx.Field("method", info.FullMethod))
            start := time.Now()
            defer func() {
				logx.Info("RPC请求处理完成", 
					logx.Field("method", info.FullMethod), 
					logx.Field("duration", time.Since(start)),
				)
            }()

			// 添加追踪拦截器
			tr := otel.Tracer("account.rpc")
			logx.Debugf("创建新的span: %s", info.FullMethod)
			ctx, span := tr.Start(ctx, info.FullMethod,
                trace.WithAttributes(
                    attribute.String("rpc.service", "account"),
                    attribute.String("rpc.method", info.FullMethod),
                ),
            )
			logx.Debugf("创建Span >> TraceID:%s", span.SpanContext().TraceID())
			defer func() {
				span.End()
				logx.Debugf("结束Span >> TraceID:%s", span.SpanContext().TraceID())
				// 在拦截器结束时强制立即导出
				if err := tp.ForceFlush(context.Background()); err != nil {
					logx.Error("强制刷新追踪数据失败", logx.Field("error", err))
				}
			}()

		    // 记录自定义属性
		    span.SetAttributes(
			    attribute.String("rpc.method", info.FullMethod),
		    )

            return handler(ctx, req)
		}),
	)

	logx.Infof("Add options to grpc server successfully")

	defer s.Stop()

	// 配置日志系统：使用Zap实现结构化日志输出
	// 统一设置日志级别 只需设置 logx 级别即可（zlog 会自动同步）
	// zlog 包	已经实现 logx.Writer 接口	封装了 Zap 到 logx 的适配逻辑
	logx.SetLevel(logx.DebugLevel)
	logx.SetWriter(logger.NewZapWriter(logger.GetZapLogger()))
	logx.Infof("Zap 实际级别: %v", logger.GetZapLogger().Level()) 
	logx.Debug("这是DEBUG级别测试日志") 
	logx.Info("这是INFO级别测试日志") 
	
	logx.Infof("Starting rpc server at %s...\n", c.ListenOn)

	// 启动gRPC服务器
	s.Start()
}

// 自定义 span processor 实现
// 添加了线程安全的 startTimes map 来跟踪 span 的开始时间
// 使用 Info 级别的日志替代 Debug，确保可以看到日志输出
// 使用工厂方法创建处理器实例
type logSpanProcessor struct{
	mu sync.Mutex
    startTimes map[trace.SpanID]time.Time
}

func NewLogSpanProcessor() *logSpanProcessor {
    return &logSpanProcessor{
        startTimes: make(map[trace.SpanID]time.Time),
    }
}

func (l *logSpanProcessor) OnStart(ctx context.Context, s sdktrace.ReadWriteSpan) {
    logx.Info("INFO级别日志") 

    l.mu.Lock()
    l.startTimes[s.SpanContext().SpanID()] = time.Now()
    l.mu.Unlock()
    
    logx.Infow("Span started",
        logx.Field("operation", s.Name()),
        logx.Field("traceID", s.SpanContext().TraceID().String()),
        logx.Field("spanID", s.SpanContext().SpanID().String()),
		logx.Field("timestamp", time.Now().UnixNano()),
    )
}

func (l *logSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	// Add debug logging to verify the method is called
	logx.Debugw("Span processor OnEnd called", 
		logx.Field("operation", s.Name()),
		logx.Field("traceID", s.SpanContext().TraceID().String()),
		logx.Field("spanID", s.SpanContext().SpanID().String()),
    )

    l.mu.Lock()
    startTime, exists := l.startTimes[s.SpanContext().SpanID()]
    delete(l.startTimes, s.SpanContext().SpanID())
    l.mu.Unlock()

	if !exists {
        logx.Errorw("No start time found for span",
            logx.Field("spanID", s.SpanContext().SpanID().String()))
        return
    }
    duration := time.Since(startTime)
    // duration := time.Since(l.startTime)
	
	// Enhanced span logging with more details	
	logx.Infow("Span completed",
		logx.Field("name", s.Name()),
		logx.Field("traceID", s.SpanContext().TraceID().String()),
		logx.Field("spanID", s.SpanContext().SpanID().String()),
		logx.Field("parentSpanID", s.Parent().SpanID().String()),
		logx.Field("duration_ms", duration.Milliseconds()),
		// logx.Field("duration", s.EndTime().Sub(s.StartTime())),
		logx.Field("attributes", s.Attributes()),
		logx.Field("events", s.Events()),
		logx.Field("status", s.Status()),
		logx.Field("isSampled", s.SpanContext().IsSampled()),
	)

	// 性能监控 - 记录慢span
    if duration > 500*time.Millisecond {
        logx.Sloww("检测到慢span",
            logx.Field("operation", s.Name()),
            logx.Field("duration_ms", duration.Milliseconds()),
        )
    }
}

func (l *logSpanProcessor) Shutdown(ctx context.Context) error {
    l.mu.Lock()
    defer l.mu.Unlock()
    l.startTimes = make(map[trace.SpanID]time.Time)
    logx.Info("关闭span处理器")
    return nil
}

func (l *logSpanProcessor) ForceFlush(ctx context.Context) error {
    return nil
}