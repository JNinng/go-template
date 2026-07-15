// Package nacos 提供 Nacos 配置中心的 Source 实现和服务注册功能
package nacos

import (
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
)

// newClientConfig 构造共享的 Nacos 客户端连接配置
//
// 同时被配置中心 (source.go) 和服务注册 (registrar.go) 使用，
// 确保所有 Nacos 子系统的连接参数保持一致。
func newClientConfig(addr string, port uint64, username, password, namespace, logLevel, logDir, cacheDir string) (*constant.ClientConfig, []constant.ServerConfig) {
	clientCfg := constant.NewClientConfig(
		constant.WithUsername(username),
		constant.WithPassword(password),
		constant.WithNamespaceId(namespace),
		constant.WithLogLevel(logLevel),
		constant.WithLogDir(logDir),
		constant.WithCacheDir(cacheDir),
		constant.WithNotLoadCacheAtStart(true),
	)
	serverCfgs := []constant.ServerConfig{
		*constant.NewServerConfig(addr, port),
	}
	return clientCfg, serverCfgs
}
