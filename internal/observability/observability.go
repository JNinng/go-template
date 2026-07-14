package observability

import (
	"context"
	"net/http"
	"time"

	"go-template/internal/config"
	"go-template/internal/logger"
	"go-template/internal/observability/health"
	"go-template/internal/observability/logs"
	"go-template/internal/observability/metrics"
	"go-template/internal/observability/tracing"

	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
)

const shutdownTimeout = 5 * time.Second

// ─── 回调注册类型 ───

// RegisterFunc 接收配置的路径和已初始化的 health、metrics http.Handler，
// 由调用方决定如何注册到任意 HTTP 框架。
//
// 使用示例:
//
//	observability.WithRegistrar(func(healthPath, metricsPath string, health, metrics http.Handler) {
//	    app.Get(healthPath, adaptor.HTTPHandler(health))
//	    app.Get(metricsPath, adaptor.HTTPHandler(metrics))
//	})
type RegisterFunc func(healthPath, metricsPath string, health, metrics http.Handler)

// ─── OpenTelemetry 独立初始化 ───

// InitOTel 仅初始化 OpenTelemetry（tracing + logs），不涉及 HTTP 路由。
//
// 适用于用户已有自己的 HTTP 框架（fiber、gin、echo 等），只需 OTel 导出管线，
// 而 health/metrics 路由由用户自行注册的场景。
//
// 使用示例:
//
//	otelShutdown, err := observability.InitOTel(ctx, cfg.Observability.OTel)
//	if err != nil {
//	    logger.Warn("OTel init failed", zap.Error(err))
//	}
//	defer func() { otelShutdown(context.Background()) }()
func InitOTel(ctx context.Context, cfg config.OTelConfig) (shutdown func(context.Context) error) {
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

// ─── 功能选项 ───

// startConfig 内部选项配置
type startConfig struct {
	registerFn RegisterFunc // 非 nil 时回调该函数，而非启动独立 HTTP Server
}

// StartOption Start 函数的功能选项
type StartOption func(*startConfig)

// WithRegistrar 将已初始化的 health 和 metrics http.Handler 通过回调交给调用方，
// 由调用方自行注册到任意 HTTP 框架，而不是启动独立的 HTTP Server。
//
// 使用示例:
//
//	// fiber
//	observability.Start(ctx, cfg, observability.WithRegistrar(
//	    func(healthPath, metricsPath string, health, metrics http.Handler) {
//	        app.Get(healthPath, adaptor.HTTPHandler(health))
//	        app.Get(metricsPath, adaptor.HTTPHandler(metrics))
//	    },
//	))
//
//	// gin
//	observability.WithRegistrar(func(healthPath, metricsPath string, health, metrics http.Handler) {
//	    router.GET(healthPath, gin.WrapH(health))
//	    router.GET(metricsPath, gin.WrapH(metrics))
//	})
//
//	// net/http
//	observability.WithRegistrar(func(healthPath, metricsPath string, health, metrics http.Handler) {
//	    mux.Handle(healthPath, health)
//	    mux.Handle(metricsPath, metrics)
//	})
func WithRegistrar(fn RegisterFunc) StartOption {
	return func(c *startConfig) {
		c.registerFn = fn
	}
}

// Start 启动可观测性组件（OTel + HTTP Server 一键启动）。
//
// 内部调用 InitOTel 初始化 tracing/logs，然后根据选项设置 HTTP 路由。
// 如果只需要 OTel 而不需要自动启动 HTTP Server，请直接使用 InitOTel。
//
// 两个独立开关:
//   - cfg.Enabled: 控制 metrics/health HTTP Server（或路由注册）
//   - cfg.OTel.Enabled: 控制 OpenTelemetry 初始化（tracing + logs）
//
// 当 cfg.Enabled 为 true 时：
//   - 如果通过 WithRegistrar 传入了回调，调用回调将 handler 交给调用方
//   - 否则在 cfg.Addr 上启动独立 HTTP Server
func Start(ctx context.Context, cfg config.ObservabilityConfig, opts ...StartOption) error {
	var sc startConfig
	for _, o := range opts {
		o(&sc)
	}

	// OpenTelemetry 初始化
	otelShutdown := InitOTel(ctx, cfg.OTel)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		otelShutdown(shutdownCtx)
	}()

	// HTTP Server / 路由注册
	if !cfg.Enabled {
		return nil
	}

	healthHandler := health.NewHandler()
	metricsHandler := metrics.Handler()

	if sc.registerFn != nil {
		// 回调模式：将路径和 handler 交给调用方自行注册
		sc.registerFn(cfg.HealthPath, cfg.MetricsPath, healthHandler, metricsHandler)
		logger.Info("Observability handlers delivered via WithRegistrar callback")
		return nil
	}

	// 默认：启动独立 HTTP Server
	if cfg.Addr == "" {
		return nil
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.HealthPath, healthHandler)
	mux.Handle(cfg.MetricsPath, metricsHandler)

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	go func() {
		logger.Infof("Observability HTTP server starting on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warnf("Observability HTTP server error: %v", err)
		}
	}()

	return nil
}

// ─── 基础 Handler（供手动注册） ───

// HealthHandler 返回 health 检查 Handler（实现 http.Handler）。
//
// 可直接注册到任意 HTTP 框架：
//
//	// net/http
//	mux.Handle("/health", observability.HealthHandler())
//
//	// fiber
//	app.Get("/health", adaptor.HTTPHandler(observability.HealthHandler()))
//
//	// gin
//	router.GET("/health", gin.WrapH(observability.HealthHandler()))
//
//	// echo
//	router.GET("/health", echo.WrapHandler(observability.HealthHandler()))
func HealthHandler() *health.Handler {
	return health.NewHandler()
}

// MetricsHandler 返回 Prometheus metrics Handler（实现 http.Handler）。
//
// 可直接注册到任意 HTTP 框架。用法与 HealthHandler 相同。
func MetricsHandler() http.Handler {
	return metrics.Handler()
}
