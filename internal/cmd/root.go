package cmd

import (
	"context"
	"fmt"
	"os"

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

		cfg, err := config.Init(configPath)
		if err != nil {
			return fmt.Errorf("config init: %w", err)
		}

		lc := cfg.LoggerConfig()
		if err := logger.Init(&lc); err != nil {
			return fmt.Errorf("logger init: %w", err)
		}

		config.AddWatch(func(newCfg, oldCfg *config.Config) {
			if newCfg.Log != oldCfg.Log {
				newLc := newCfg.LoggerConfig()
				if err := logger.Reset(&newLc); err != nil {
					logger.Error("Failed to reset logger", zap.Error(err))
				}
			}
		})

		ctx := signal.ContextWithShutdown(context.Background())

		if cfg.Observability.Enabled {
			if err := observability.Start(ctx, cfg.Observability); err != nil {
				logger.Warnf("Failed to start observability: %v", err)
			}
		}

		// 自动注册到 Nacos
		if cfg.NacosService.Enabled {
			registrar, err := nacos.NewRegistrar(&cfg.NacosService)
			if err != nil {
				logger.Warnf("Failed to create nacos registrar: %v", err)
			} else {
				if err := registrar.Register(); err != nil {
					logger.Warnf("Failed to register service with nacos: %v", err)
				} else {
					defer registrar.Deregister()
					defer registrar.Close()
					logger.Info("Service registered with Nacos",
						zap.String("service", cfg.NacosService.ServiceName),
						zap.String("ip", cfg.NacosService.ServiceIP),
						zap.Uint64("port", cfg.NacosService.ServicePort),
					)
				}
			}
		}

		logger.Info("Application initialized",
			zap.String("name", cfg.App.Name),
			zap.String("env", cfg.App.Env),
		)

		if err := app.Run(ctx); err != nil {
			logger.Error("Application error", zap.Error(err))
		}

		logger.Info("Cleaning up resources...")
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
