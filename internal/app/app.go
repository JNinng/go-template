package app

import (
	"context"
	"go-template/internal/config"
	"go-template/internal/logger"
	"go-template/internal/observability"
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

	<-ctx.Done()

	logger.Info("Business logic shutting down")
	return nil
}
