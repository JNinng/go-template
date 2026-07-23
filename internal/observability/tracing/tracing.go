package tracing

import (
	"context"
	"fmt"

	"go-template/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init 初始化 OpenTelemetry Tracing。
// 返回的 shutdown 函数应在应用退出前调用以 flush 未发送的 span。
//
// 当 Traces.Enabled 为 true 时，始终创建 TracerProvider 并通过
// otel.SetTracerProvider 注册到全局，确保应用具备 trace ID 生成能力
// 以支持日志关联和上下文传播。
// 只有当 Endpoint 非空时，才会创建导出器将 span 发送到 OTLP collector。
func Init(ctx context.Context, cfg config.OTelConfig, res *resource.Resource) (func(context.Context) error, error) {
	if !cfg.Traces.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	tpOpts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
	}

	if cfg.Endpoint != "" {
		var exporter sdktrace.SpanExporter
		var err error

		switch cfg.Protocol {
		case "http":
			exporter, err = otlptracehttp.New(ctx,
				otlptracehttp.WithEndpoint(cfg.Endpoint),
				otlptracehttp.WithInsecure(),
			)
		default:
			exporter, err = otlptracegrpc.New(ctx,
				otlptracegrpc.WithEndpoint(cfg.Endpoint),
				otlptracegrpc.WithInsecure(),
			)
		}
		if err != nil {
			return nil, fmt.Errorf("create trace exporter: %w", err)
		}

		tpOpts = append(tpOpts, sdktrace.WithBatcher(exporter))
	}

	tp := sdktrace.NewTracerProvider(tpOpts...)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
