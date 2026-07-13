// Package nacos 提供 Nacos 配置中心的 Source 实现和服务注册功能
package nacos

import (
	"errors"
	"strings"
	"sync"

	"go-template/internal/config"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/model"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// SubscribeCallback 服务变更回调函数类型
type SubscribeCallback func(services []model.Instance, err error)

// Registrar Nacos 服务注册器
//
// 封装 Nacos Naming 客户端，提供注册、注销、发现和订阅功能。
// 通过 NewRegistrar 创建实例，使用完毕后调用 Close 清理资源。
type Registrar struct {
	cfg          *config.NacosServiceConfig
	client       naming_client.INamingClient
	subCallbacks map[string]SubscribeCallback
	subMu        sync.Mutex
}

// subKey builds a map key from service name and clusters.
func subKey(serviceName string, clusters []string) string {
	return serviceName + "\x00" + strings.Join(clusters, ",")
}

// NewRegistrar 创建一个 Nacos 服务注册器
//
// 参数:
//   - cfg: Nacos 服务注册配置
//
// 返回值:
//   - *Registrar: 注册器实例
//   - error: 创建失败时返回错误
func NewRegistrar(cfg *config.NacosServiceConfig) (*Registrar, error) {
	if cfg == nil {
		return nil, errors.New("nacos service config is nil")
	}

	clientCfg := constant.NewClientConfig(
		constant.WithUsername(cfg.Username),
		constant.WithPassword(cfg.Password),
		constant.WithNamespaceId(cfg.Namespace),
		constant.WithLogLevel(cfg.LogLevel),
		constant.WithLogDir(cfg.LogDir),
		constant.WithCacheDir(cfg.CacheDir),
		constant.WithNotLoadCacheAtStart(true),
	)
	serverCfgs := []constant.ServerConfig{
		*constant.NewServerConfig(cfg.Addr, cfg.Port),
	}

	client, err := clients.NewNamingClient(vo.NacosClientParam{
		ClientConfig:  clientCfg,
		ServerConfigs: serverCfgs,
	})
	if err != nil {
		return nil, err
	}

	return &Registrar{
		cfg:          cfg,
		client:       client,
		subCallbacks: make(map[string]SubscribeCallback),
	}, nil
}

// Register 注册服务实例到 Nacos 并启动心跳
//
// 注册成功后，Nacos SDK 会自动维护心跳，无需额外处理。
//
// 返回值:
//   - error: 注册失败时返回错误
func (r *Registrar) Register() error {
	if r.client == nil {
		return errors.New("nacos naming client is nil")
	}

	ok, err := r.client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          r.cfg.ServiceIP,
		Port:        r.cfg.ServicePort,
		Weight:      r.cfg.Weight,
		Enable:      true,
		Healthy:     r.cfg.Healthy,
		Metadata:    r.cfg.Metadata,
		ClusterName: r.cfg.ClusterName,
		ServiceName: r.cfg.ServiceName,
		GroupName:   r.cfg.Group,
		Ephemeral:   true,
	})
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("register instance failed: returned false")
	}
	return nil
}

// Deregister 从 Nacos 注销服务实例
//
// 返回值:
//   - error: 注销失败时返回错误
func (r *Registrar) Deregister() error {
	if r.client == nil {
		return errors.New("nacos naming client is nil")
	}

	ok, err := r.client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          r.cfg.ServiceIP,
		Port:        r.cfg.ServicePort,
		Cluster:     r.cfg.ClusterName,
		ServiceName: r.cfg.ServiceName,
		GroupName:   r.cfg.Group,
		Ephemeral:   true,
	})
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("deregister instance failed: returned false")
	}
	return nil
}

// GetService 获取指定服务的所有实例
//
// 参数:
//   - serviceName: 服务名称
//   - clusters: 集群列表，nil 表示默认集群
//
// 返回值:
//   - model.Service: 服务信息（包含实例列表）
//   - error: 查询失败时返回错误
func (r *Registrar) GetService(serviceName string, clusters []string) (model.Service, error) {
	if r.client == nil {
		return model.Service{}, errors.New("nacos naming client is nil")
	}

	return r.client.GetService(vo.GetServiceParam{
		ServiceName: serviceName,
		Clusters:    clusters,
		GroupName:   r.cfg.Group,
	})
}

// SelectOneHealthyInstance 通过加权轮询选择一个健康实例
//
// 参数:
//   - serviceName: 服务名称
//   - clusters: 集群列表，nil 表示默认集群
//
// 返回值:
//   - *model.Instance: 选中的实例
//   - error: 查询失败时返回错误
func (r *Registrar) SelectOneHealthyInstance(serviceName string, clusters []string) (*model.Instance, error) {
	if r.client == nil {
		return nil, errors.New("nacos naming client is nil")
	}

	return r.client.SelectOneHealthyInstance(vo.SelectOneHealthInstanceParam{
		ServiceName: serviceName,
		Clusters:    clusters,
		GroupName:   r.cfg.Group,
	})
}

// Subscribe 订阅服务变更通知
//
// 当服务实例列表发生变化时，会调用回调函数。
//
// 参数:
//   - serviceName: 服务名称
//   - clusters: 集群列表，nil 表示默认集群
//   - cb: 变更回调函数
//
// 返回值:
//   - error: 订阅失败时返回错误
func (r *Registrar) Subscribe(serviceName string, clusters []string, cb SubscribeCallback) error {
	if r.client == nil {
		return errors.New("nacos naming client is nil")
	}

	err := r.client.Subscribe(&vo.SubscribeParam{
		ServiceName:       serviceName,
		Clusters:          clusters,
		GroupName:         r.cfg.Group,
		SubscribeCallback: cb,
	})
	if err != nil {
		return err
	}

	r.subMu.Lock()
	r.subCallbacks[subKey(serviceName, clusters)] = cb
	r.subMu.Unlock()
	return nil
}

// Unsubscribe 取消服务变更订阅
//
// 参数:
//   - serviceName: 服务名称
//   - clusters: 集群列表，nil 表示默认集群
//
// 返回值:
//   - error: 取消订阅失败时返回错误
func (r *Registrar) Unsubscribe(serviceName string, clusters []string) error {
	if r.client == nil {
		return errors.New("nacos naming client is nil")
	}

	key := subKey(serviceName, clusters)
	r.subMu.Lock()
	cb, ok := r.subCallbacks[key]
	if ok {
		delete(r.subCallbacks, key)
	}
	r.subMu.Unlock()

	unsubCb := cb
	if !ok {
		unsubCb = func(_ []model.Instance, _ error) {}
	}

	return r.client.Unsubscribe(&vo.SubscribeParam{
		ServiceName:       serviceName,
		Clusters:          clusters,
		GroupName:         r.cfg.Group,
		SubscribeCallback: unsubCb,
	})
}

// Close 关闭注册器，释放底层连接资源
func (r *Registrar) Close() {
	if r.client != nil {
		r.client.CloseClient()
	}
}
