package test

import (
	"context"
	"log"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// go test -v -run ^TestOtel$ common/test/otel_test.go
// go test -v -run TestOtel ./common/test/...
func TestOtel(t *testing.T) {

	// OTLP初始化
    tp, err := initTracer()
    if err != nil {
        t.Error("初始化追踪器失败", err)
        return
    }
    defer tp.Shutdown(context.Background())

	otel.SetTracerProvider(tp)
    otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
        propagation.TraceContext{},
        propagation.Baggage{},
    ))

	ctx := context.Background()
	tr := tp.Tracer("test-tracer")
	ctx, span := tr.Start(ctx, "test-span")
	<-time.After(100 * time.Millisecond)
	span.End()
	tp.ForceFlush(ctx)

	if span.SpanContext().TraceID().IsValid() {
        t.Logf("追踪数据已生成，TraceID: %s", span.SpanContext().TraceID())
    } else {
        t.Error("生成的TraceID无效")
    }
}


func initTracer() (*sdktrace.TracerProvider, error) {
    exporter, err := otlptracegrpc.New(context.Background(),
        otlptracegrpc.WithEndpoint("jaeger:4317"),
        otlptracegrpc.WithInsecure(),
    )
	if err != nil {
        log.Fatalf("创建导出器失败: %v", err)
		return nil, err
	}
    
	log.Printf("默认资源属性: %v", resource.Default())
    res, _ := resource.Merge(
        resource.Default(),
        resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceName("test-service"), 
        ),
    )
    
	log.Println("正在初始化追踪器...")
    tp := sdktrace.NewTracerProvider(
        sdktrace.WithBatcher(exporter),
        sdktrace.WithResource(res),
    )
    
    return tp, nil
}