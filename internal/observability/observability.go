package observability

import (
	"context"
	"net/http"

	"go-template/internal/config"
	"go-template/internal/logger"
	"go-template/internal/observability/health"
	"go-template/internal/observability/logs"
	"go-template/internal/observability/metrics"
	"go-template/internal/observability/tracing"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

// ─── OpenTelemetry 初始化 ───

// InitOTel 初始化 OpenTelemetry（tracing + logs），不涉及 HTTP 路由。
//
// 返回统一签名的 shutdown 函数，由调用方 push 到 shutdown 栈。
//
// 使用示例:
//
//	otelShutdown := observability.InitOTel(ctx, cfg.Observability.OTel)
//	push(otelShutdown)
func InitOTel(ctx context.Context, cfg config.OTelConfig) func(context.Context) error {
	if !cfg.Enabled {
		return func(context.Context) error { return nil }
	}

	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName(config.Get().App.Name),
		semconv.DeploymentEnvironmentName(config.Get().App.Env),
	)

	traceShutdown, err := tracing.Init(ctx, cfg, res)
	if err != nil {
		logger.Warnf("Failed to init tracing: %v", err)
	}

	logCore, logShutdown, err := logs.Init(ctx, cfg, res)
	if err != nil {
		logger.Warnf("Failed to init OTel logs: %v", err)
	}
	if logCore != nil {
		if err := logger.AddCore(logCore); err != nil {
			logger.Warnf("Failed to add OTel log core: %v", err)
		}
	}

	return func(ctx context.Context) error {
		if logShutdown != nil {
			logShutdown(ctx)
		}
		if traceShutdown != nil {
			traceShutdown(ctx)
		}
		return nil
	}
}

// ─── Observability HTTP Server ───

// StartServer 启动独立的 health/metrics HTTP Server（双端口模式）。
//
// 返回统一签名的 shutdown 函数，由调用方 push 到 shutdown 栈。
// 当 cfg.Enabled 为 false 或 cfg.Addr 为空时，返回 no-op shutdown。
//
// 使用示例:
//
//	srvShutdown, err := observability.StartServer(ctx, cfg.Observability)
//	if err != nil { ... }
//	push(srvShutdown)
func StartServer(_ context.Context, cfg config.ObservabilityConfig) (func(context.Context) error, error) {
	if !cfg.Enabled || cfg.Addr == "" {
		return func(context.Context) error { return nil }, nil
	}

	healthHandler := health.NewHandler()
	metricsHandler := metrics.Handler()

	mux := http.NewServeMux()
	mux.Handle(cfg.HealthPath, healthHandler)
	mux.Handle(cfg.MetricsPath, metricsHandler)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}

	go func() {
		logger.Infof("Observability HTTP server starting on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warnf("Observability HTTP server error: %v", err)
		}
	}()

	return func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	}, nil
}

// ─── 基础 Handler（供单端口模式手动注册） ───

// HealthHandler 返回 health 检查 Handler（实现 http.Handler）。
//
// 单端口模式下，由 root.go 创建后通过 app.Deps 注入业务 server：
//
//	// fiber
//	app.Get("/health", adaptor.HTTPHandler(deps.HealthHandler))
//
//	// gin
//	router.GET("/health", gin.WrapH(deps.HealthHandler))
//
//	// net/http
//	mux.Handle("/health", deps.HealthHandler)
func HealthHandler() *health.Handler {
	return health.NewHandler()
}

// MetricsHandler 返回 Prometheus metrics Handler（实现 http.Handler）。
//
// 用法与 HealthHandler 相同。
func MetricsHandler() http.Handler {
	return metrics.Handler()
}
