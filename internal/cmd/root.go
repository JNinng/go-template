package cmd

import (
	"context"
	"fmt"
	"os"

	"go-template/internal/app"
	"go-template/internal/config"
	"go-template/internal/logger"
	"go-template/internal/nacos"
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

		// Nacos 配置中心 — 从远程拉取配置
		if cfg.Nacos.Enabled {
			if err := config.MergeSource(nacos.NewSource(&cfg.Nacos)); err != nil {
				logger.Warnf("Failed to init nacos config source: %v", err)
			}
			cfg = config.Get() // MergeSource 后刷新，获取合并后的配置
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
		defer config.CloseSources() // 确保外部配置源在 shutdown 时正确关闭

		// Nacos 服务注册 — 将本实例注册到 Nacos
		if cfg.NacosService.Enabled {
			registrar, err := nacos.NewRegistrar(&cfg.NacosService)
			if err != nil {
				logger.Warnf("Failed to create nacos registrar: %v", err)
			} else {
				defer registrar.Close() // 确保客户端连接在 shutdown 时关闭
				if err := registrar.Register(); err != nil {
					logger.Warnf("Failed to register service with nacos: %v", err)
				} else {
					defer registrar.Deregister() // 仅成功注册后才注销
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
