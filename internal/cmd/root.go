package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"go-template/internal/app"
	"go-template/internal/config"
	"go-template/internal/logger"
	"go-template/internal/nacos"
	"go-template/internal/observability"
	"go-template/internal/signal"

	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func defaultConfigPath() string {
	if _, err := os.Stat("configs/config.yaml"); err == nil {
		return "configs/config.yaml"
	}
	return "config.yaml"
}

var rootCmd = &cobra.Command{
	Use:   "app",
	Short: "Go service template",
	RunE: func(cmd *cobra.Command, _ []string) error {
		configPath, _ := cmd.Flags().GetString("config")

		// ─── 初始化（顺序即依赖） ───

		cfg, err := config.Init(configPath)
		if err != nil {
			return fmt.Errorf("config init: %w", err)
		}

		lc := cfg.LoggerConfig()
		if err := logger.Init(&lc); err != nil {
			return fmt.Errorf("logger init: %w", err)
		}

		// Nacos 配置中心 — 从远程拉取配置
		if cfg.Nacos.Enabled {
			if err := config.MergeSource(nacos.NewSource(&cfg.Nacos)); err != nil {
				logger.Warnf("Failed to init nacos config source: %v", err)
			}
			cfg = config.Get()
		}

		config.AddWatch(func(newCfg, oldCfg *config.Config) {
			if newCfg.Log != oldCfg.Log {
				newLc := newCfg.LoggerConfig()
				if err := logger.Reset(&newLc); err != nil {
					logger.Error("Failed to reset logger", zap.Error(err))
				}
			}
		})

		// ─── shutdown 栏（逆序清理） ───

		var cleanups []func(context.Context) error
		push := func(fn func(context.Context) error) {
			cleanups = append(cleanups, fn)
		}

		ctx := signal.ContextWithShutdown(context.Background())

		// Config sources
		push(func(context.Context) error { config.CloseSources(); return nil })

		// OTel (tracing + logs)
		otelShutdown := observability.InitOTel(ctx, cfg.Observability.OTel)
		push(otelShutdown)

		// Nacos 服务注册
		if cfg.NacosService.Enabled {
			registrar, err := nacos.NewRegistrar(&cfg.NacosService)
			if err != nil {
				logger.Warnf("Failed to create nacos registrar: %v", err)
			} else {
				if err := registrar.Register(); err != nil {
					logger.Warnf("Failed to register service with nacos: %v", err)
				} else {
					logger.Info("Service registered with Nacos",
						zap.String("service", cfg.NacosService.ServiceName),
						zap.String("ip", cfg.NacosService.ServiceIP),
						zap.Uint64("port", cfg.NacosService.ServicePort),
					)
				}
				push(registrar.Shutdown)
			}
		}

		logger.Info("Application initialized",
			zap.String("name", cfg.App.Name),
			zap.String("env", cfg.App.Env),
		)

		// ─── 运行业务 ───

		deps := app.Deps{}

		// Observability HTTP（配置驱动端口模式）
		if cfg.Observability.Enabled {
			if cfg.Observability.SinglePort {
				// 单端口：handler 注入业务 server
				deps.HealthHandler = observability.HealthHandler()
				deps.MetricsHandler = observability.MetricsHandler()
			} else {
				// 双端口：起独立 HTTP server
				srvShutdown, err := observability.StartServer(ctx, cfg.Observability)
				if err != nil {
					logger.Warn("Failed to start observability server", zap.Error(err))
				} else {
					push(srvShutdown)
				}
			}
		}

		if err := app.Run(ctx, deps); err != nil {
			logger.Error("Application error", zap.Error(err))
		}

		// ─── 逆序清理 ───

		logger.Info("Cleaning up resources...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for i := len(cleanups) - 1; i >= 0; i-- {
			if err := cleanups[i](shutdownCtx); err != nil {
				logger.Warn("Cleanup error", zap.Int("index", i), zap.Error(err))
			}
		}
		logger.Sync()
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("config", "c", defaultConfigPath(), "Config file path")
}
