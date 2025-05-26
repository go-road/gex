package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
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
	"google.golang.org/grpc/metadata"
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
func initTracer(c config.Config) (*sdktrace.TracerProvider, error) {
	if os.Getenv("OTEL_LOG_LEVEL") == "debug" {
    	logx.Info("OpenTelemetry 调试模式已启用")
	}
	fmt.Println("=== 这是直接控制台输出测试 ===")
	// 优先使用环境变量
    endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
    if endpoint == "" {
        endpoint = c.OTLP.Endpoint
    }
	if endpoint == "" {
		endpoint = "jaeger:4317" 
        // return nil, fmt.Errorf("OTLP endpoint未配置，请检查环境变量OTEL_EXPORTER_OTLP_ENDPOINT或配置文件中的otlp.endpoint")
    }
    logx.Infof("正在连接Jaeger端点: %s", endpoint) 
    
    // 创建带超时的context 使用独立上下文用于连接检查
    connCtx, connCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer connCancel()

    // 添加预连接检查
    conn, err := grpc.DialContext(connCtx, endpoint, 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), // 阻塞直到连接成功
		grpc.WithReturnConnectionError(), // 获取详细错误
	)
    if err != nil {
        return nil, fmt.Errorf("无法连接Jaeger: %v", err)
    }
    defer func() {
		if err := conn.Close(); err != nil {
			logx.Error("关闭预连接失败", logx.Field("error", err))
		}
	}()
	state := conn.GetState()
	logx.Infof("Jaeger实际连接地址: %s", conn.Target())
	logx.Infof("Jaeger连接状态: %s", state.String())
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
	// 构建配置选项
    var opts []otlptracegrpc.Option
    opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
    if c.OTLP.Insecure {
        opts = append(opts, otlptracegrpc.WithInsecure())
    }
	if c.OTLP.Timeout > 0 {
		opts = append(opts, otlptracegrpc.WithTimeout(time.Duration(c.OTLP.Timeout)*time.Second))
	}
	opts = append(opts, 
		otlptracegrpc.WithRetry(otlptracegrpc.RetryConfig{
			Enabled:         true,
			InitialInterval: 1 * time.Second,
			MaxInterval:     5 * time.Second,
			MaxElapsedTime:  30 * time.Second,
		}))
	// 创建导出器时同时考虑代码配置和环境变量配置
	logx.Debugf("导出器上下文ID: %p", context.Background())		
	exporter, err := otlptracegrpc.New(
		context.Background(), // 应用生命周期级别的上下文
		opts...,
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
			semconv.ServiceName("account-rpc-service"),
			semconv.ServiceVersion("v1.0.0"),
			semconv.DeploymentEnvironment("prod"),
			attribute.String("host.name", "account-rpc"),
			attribute.String("service.instance.id", os.Getenv("HOSTNAME")), // 添加实例标识
    		attribute.String("exporter", "jaeger"),     
			// semconv.ServiceNameKey.String("account.rpc"), // 强制指定服务名
            // attribute.String("service.version", "v1.2.3"),
			// attribute.String("environment", "prod"),
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

	// 使用官方BatchSpanProcessor 官方处理器会自动处理以下内容：
	// 1.批量收集Span数据
	// 2.自动重试机制
	// 3.内存队列管理
	// 4.异步导出优化
    batchProcessor := sdktrace.NewBatchSpanProcessor(exporter, 
        sdktrace.WithBatchTimeout(5*time.Second),
        sdktrace.WithMaxExportBatchSize(512),
        sdktrace.WithMaxQueueSize(2048),
        sdktrace.WithExportTimeout(10*time.Second),
    )
	logx.Info("BatchSpanProcessor创建成功", logx.Field("batchProcessor", batchProcessor))

	// 日志导出器（仅用于调试）
	debugExporter, err := stdouttrace.New(
		stdouttrace.WithPrettyPrint(),
		stdouttrace.WithWriter(os.Stdout), // 直接输出到标准输出
		stdouttrace.WithWriter(&logWriter{logger: logx.Info}),  // 同时记录到日志系统
	)
	if err != nil {
    	logx.Error("创建stdout导出器失败", logx.Field("error", err))
	}
	logx.Info("创建stdout导出器成功", logx.Field("debugExporter", debugExporter))

	logx.Info("资源信息配置成功,正在创建TracerProvider...")
	// 创建TracerProvider
	// 1.双重输出机制：批处理器每5秒批量发送到Jaeger，简单处理器实时打印到控制台
	// 2.调试生产两不误：开发时可查看实时日志，生产环境保持批量发送的高效性
	// 3.独立工作互不干扰：两个处理器分别处理Span数据，不会互相阻塞
	// 4.生产环境建议移除SimpleSpanProcessor，避免性能损耗
	tp := sdktrace.NewTracerProvider(
		// sdktrace.WithBatcher(exporter, 
        //     sdktrace.WithBatchTimeout(5*time.Second),
		// 	sdktrace.WithMaxExportBatchSize(512),
        //     sdktrace.WithMaxQueueSize(2048),
        //     sdktrace.WithExportTimeout(10*time.Second),
        // ),
		// sdktrace.WithSyncer(exporter), // 同步导出
		sdktrace.WithSpanProcessor(batchProcessor), // 批量处理器（用于生产环境）
		// sdktrace.WithSpanProcessor(sdktrace.NewSimpleSpanProcessor(debugExporter)), // 即时输出处理器（用于调试）
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)	
	logx.Info("TracerProvider创建成功")

	// 注册自定义处理器用于调试
	// logx.Info("注册自定义span处理器")
	// tp.RegisterSpanProcessor(&logSpanProcessor{})
	// tp.RegisterSpanProcessor(NewLogSpanProcessor())
	// processor := NewLogSpanProcessor()
	// if processor == nil {
	// 	return tp, fmt.Errorf("failed to create span processor")
	// }
	// tp.RegisterSpanProcessor(processor)

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

	// 打印完整配置树
    logx.Infof("完整配置结构: %+v", c) 
    // 专门打印OTLP配置
    logx.Infow("OTLP配置详情",
        logx.Field("endpoint", c.OTLP.Endpoint),
        logx.Field("insecure", c.OTLP.Insecure),
        logx.Field("timeout", c.OTLP.Timeout),
        logx.Field("export_type", c.OTLP.ExportType),
    )
	// 添加注册验证日志
	logx.Infof("ETCD registration config: %+v", c.RpcServerConf.Etcd)
    logx.Infof("RpcServerConf.Etcd.Hosts = %v", c.RpcServerConf.Etcd.Hosts)
    logx.Infof("RpcServerConf.Etcd.Key = %s", c.RpcServerConf.Etcd.Key)
	logx.Infof("当前OTLP配置: %+v", c.OTLP)
    logx.Infof("环境变量OTEL_EXPORTER_OTLP_ENDPOINT: %s", os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))

	// 初始化追踪器
	tp, err := initTracer(c)
	if err != nil {
		logx.Error("初始化追踪器失败", logx.Field("error", err))
		return
	}
	logx.Info("OpenTelemetry追踪器已初始化", 
    logx.Field("service", "account-rpc-service"),
    logx.Field("endpoint", "jaeger:4318"))
	// defer tp.Shutdown(context.Background())
	defer func() {
        if err := tp.Shutdown(context.Background()); err != nil {
            logx.Error("关闭追踪器失败", logx.Field("error", err))
        }
    }()

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
        grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(tp), // 使用自定义的TracerProvider
			otelgrpc.WithPropagators(otel.GetTextMapPropagator()), // 使用全局传播器
		))  
        // grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor())

		// 注册protobuf定义的gRPC服务实现
		pb.RegisterAccountServiceServer(grpcServer, server.NewAccountServiceServer(ctx))
		logx.Infof("c.Mode is %+v , service.DevMode is %+v, service.TestMode is %+v", c.Mode, service.DevMode, service.TestMode)
		// 开发/测试/生产模式启用gRPC反射服务（用于grpcurl调试）
		if c.Mode == service.DevMode || c.Mode == service.TestMode  || c.Mode == service.ProMode {
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
		// 拦截器上下文（请求级）
		grpc.UnaryInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		    // 添加追踪拦截器前需要先提取上下文
			logx.Debugf("拦截器上下文ID: %p", ctx)
			ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(metadata.New(nil)))
  
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
			tr := otel.Tracer("account-tracer")
			logx.Debugf("创建新的span: %s", info.FullMethod)
			// 创建span时需要携带正确的上下文 使用请求上下文创建span
			ctx, span := tr.Start(ctx, info.FullMethod,
                trace.WithAttributes(
                    attribute.String("rpc.service", "account"),
                    attribute.String("rpc.method", info.FullMethod),
                ),
            )
			logx.Debugf("创建Span >> TraceID:%s", span.SpanContext().TraceID())
			defer func() {
				span.End()
				if span.SpanContext().TraceID().IsValid() {
					logx.Debugf("追踪数据已生成，TraceID: %s", span.SpanContext().TraceID())
				} else {
					logx.Error("生成的TraceID无效")
				}
				// 异步刷新避免阻塞请求
				go func() {
					logx.Debugf("结束Span >> TraceID:%s", span.SpanContext().TraceID())
					ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					defer cancel()
					// 在拦截器结束时强制立即导出
					if err := tp.ForceFlush(ctx); err != nil {
						logx.Error("强制刷新追踪数据失败", logx.Field("error", err))
					}
				}()
			}()

		    // 记录自定义属性
		    span.SetAttributes(
			    attribute.String("rpc.method", info.FullMethod),
		    )

			// 必须将新上下文传递给后续处理
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

// 添加自定义日志写入器
type logWriter struct {
    logger func(...interface{})
}

func (l *logWriter) Write(p []byte) (n int, err error) {
    l.logger("OpenTelemetry Span Output: ", string(p))
    return len(p), nil
}