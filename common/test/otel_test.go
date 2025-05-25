package test

import (
	"context"
	"log"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

// go test -v -run ^TestOtel$ common/test/otel_test.go
// go test -v -run TestOtel ./common/test/...
func TestOtel(t *testing.T) {
	exporter, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithEndpoint("jaeger:4317"),
		otlptracegrpc.WithInsecure(),
	)
    if err != nil {
        log.Fatalf("创建导出器失败: %v", err)
    }

	t.Log("正在初始化追踪器...")
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("test-service"),
		)),
	)
	otel.SetTracerProvider(tp)

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