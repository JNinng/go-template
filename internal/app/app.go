package app

import (
	"context"
	"net/http"

	"go-template/internal/config"
	"go-template/internal/logger"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// Deps 由 root.go 注入的基础设施依赖（可选）。
//
// 双端口模式下字段为 nil，业务代码无需关心。
// 单端口模式下 root.go 会填充 HealthHandler / MetricsHandler，
// 业务代码将其注册到自己的 HTTP server 即可。
type Deps struct {
	HealthHandler  http.Handler
	MetricsHandler http.Handler
}

// Run 启动业务逻辑。
//
// OTel（tracing + logs）已在 root.go 基础设施层初始化完毕，
// 业务代码可直接使用 otel.Tracer() 获取 tracer。
func Run(ctx context.Context, deps Deps) error {
	cfg := config.Get()

	// 单端口模式：将 health/metrics handler 注册到业务 HTTP server
	if deps.HealthHandler != nil {
		// 示例（fiber）:
		// app.Get(cfg.Observability.HealthPath, adaptor.HTTPHandler(deps.HealthHandler))
		// app.Get(cfg.Observability.MetricsPath, adaptor.HTTPHandler(deps.MetricsHandler))
		_ = deps.MetricsHandler
	}

	if cfg.Observability.OTel.Enabled {
		var span trace.Span
		_, span = otel.Tracer(cfg.App.Name).Start(ctx, "app-run")
		traceId := span.SpanContext().TraceID().String()
		logger.InfoContext(ctx, "otel_app_run_started",
			zap.String("trace_id", traceId),
		)
		span.End()
	}

	<-ctx.Done()

	logger.InfoContext(ctx, "business_logic_shutdown_completed")
	return nil
}
