package app

import (
	"context"
	"fmt"
	"go-template/internal/config"
	"go-template/internal/logger"
	"go-template/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Run 启动业务逻辑。
//
// OTel（tracing + logs）在 root.go 的 infra 层已初始化，
func Run(ctx context.Context) error {
	cfg := config.Get()

	if cfg.Observability.Enabled {
		// 通过 WithRegistrar 回调注册 observability 路由到 fiber
		//options := observability.WithRegistrar(
		//	func(healthPath, metricsPath string, health, metrics http.Handler) {
		//		app.Get(healthPath, adaptor.HTTPHandler(health))
		//		app.Get(metricsPath, adaptor.HTTPHandler(metrics))
		//	},
		//)
		err := observability.Start(ctx, cfg.Observability)
		if err != nil {
			logger.Warn("Start observability failed", zap.Error(err))
		}
	}
	if cfg.Observability.OTel.Enabled {
		// 此时 tracing.Init 已通过 observability.Start → InitOTel 执行，
		// 全局 TracerProvider 已就绪，可以创建 span 获取 trace ID。
		var span trace.Span
		_, span = otel.Tracer("app").Start(ctx, "app-run")
		traceId := span.SpanContext().TraceID().String()
		logger.Info(fmt.Sprintf("traceId: %s", traceId))
		span.End()
	}

	<-ctx.Done()

	logger.Info("Business logic shutting down")
	return nil
}
