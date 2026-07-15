package nacos

import (
	"testing"

	"go-template/internal/config"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
)

func TestNewRegistrar(t *testing.T) {
	cfg := config.DefaultNacosServiceConfig()
	cfg.ServiceName = "test-service"
	cfg.ServicePort = 8080

	r, err := NewRegistrar(&cfg)
	if err != nil {
		t.Skipf("skipping test, no nacos server available: %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil registrar")
	}
	defer r.Close()
}

func TestNewRegistrarNilConfig(t *testing.T) {
	r, err := NewRegistrar(nil)
	if err == nil {
		t.Error("expected error with nil config")
	}
	if r != nil {
		t.Error("expected nil registrar")
	}
}

func TestNewRegistrarDefaults(t *testing.T) {
	cfg := config.DefaultNacosServiceConfig()

	// 所有字段自带独立默认值
	if cfg.Enabled {
		t.Error("expected Enabled false")
	}
	if cfg.ClusterName != "DEFAULT" {
		t.Errorf("expected ClusterName DEFAULT, got %q", cfg.ClusterName)
	}
	if cfg.Weight != 10 {
		t.Errorf("expected Weight 10, got %f", cfg.Weight)
	}
	if !cfg.Healthy {
		t.Error("expected Healthy true")
	}
	if cfg.ServiceIP != "127.0.0.1" {
		t.Errorf("expected ServiceIP 127.0.0.1, got %q", cfg.ServiceIP)
	}
	if cfg.Addr != "127.0.0.1" {
		t.Errorf("expected Addr 127.0.0.1, got %q", cfg.Addr)
	}
	if cfg.Port != 8848 {
		t.Errorf("expected Port 8848, got %d", cfg.Port)
	}
	if cfg.Group != "DEFAULT_GROUP" {
		t.Errorf("expected Group DEFAULT_GROUP, got %q", cfg.Group)
	}
	if cfg.Namespace != "public" {
		t.Errorf("expected Namespace public, got %q", cfg.Namespace)
	}

	// ServiceName 和 ServicePort 需业务配置，保持零值
	if cfg.ServiceName != "" {
		t.Errorf("expected ServiceName empty, got %q", cfg.ServiceName)
	}
	if cfg.ServicePort != 0 {
		t.Errorf("expected ServicePort 0, got %d", cfg.ServicePort)
	}

	// 零值 ServiceName/ServicePort 应被 NewRegistrar 拒绝
	r, err := NewRegistrar(&cfg)
	if err == nil {
		t.Error("expected error for missing ServiceName/ServicePort, got nil")
	}
	if r != nil {
		t.Error("expected nil registrar on validation failure")
	}
}

func TestRegisterNilClient(t *testing.T) {
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	err := r.Register()
	if err == nil {
		t.Error("expected error with nil client")
	}
}

func TestDeregisterNilClient(t *testing.T) {
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	err := r.Deregister()
	if err == nil {
		t.Error("expected error with nil client")
	}
}

func TestGetServiceNilClient(t *testing.T) {
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	_, err := r.GetService("test", nil)
	if err == nil {
		t.Error("expected error with nil client")
	}
}

func TestSelectOneHealthyInstanceNilClient(t *testing.T) {
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	_, err := r.SelectOneHealthyInstance("test", nil)
	if err == nil {
		t.Error("expected error with nil client")
	}
}

func TestSubscribeNilClient(t *testing.T) {
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	err := r.Subscribe("test", nil, func(services []model.Instance, err error) {})
	if err == nil {
		t.Error("expected error with nil client")
	}
}

func TestUnsubscribeNilClient(t *testing.T) {
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	err := r.Unsubscribe("test", nil)
	if err == nil {
		t.Error("expected error with nil client")
	}
}

func TestCloseNilClient(t *testing.T) {
	// Close with nil client should not panic
	r := &Registrar{cfg: &config.NacosServiceConfig{}}
	r.Close()
}

func TestConfigDefaultNacosServiceConfig(t *testing.T) {
	ns := config.DefaultNacosServiceConfig()

	// 所有连接字段自带独立默认值
	if ns.Addr != "127.0.0.1" {
		t.Errorf("expected Addr 127.0.0.1, got %q", ns.Addr)
	}
	if ns.Port != 8848 {
		t.Errorf("expected Port 8848, got %d", ns.Port)
	}
	if ns.Group != "DEFAULT_GROUP" {
		t.Errorf("expected Group DEFAULT_GROUP, got %q", ns.Group)
	}
	if ns.LogLevel != "debug" {
		t.Errorf("expected LogLevel debug, got %q", ns.LogLevel)
	}
	if ns.LogDir != "./logs" {
		t.Errorf("expected LogDir ./logs, got %q", ns.LogDir)
	}
	if ns.CacheDir != "./cache" {
		t.Errorf("expected CacheDir ./cache, got %q", ns.CacheDir)
	}
}

func TestSubscribeCallbackTracking(t *testing.T) {
	r := &Registrar{
		cfg:          &config.NacosServiceConfig{},
		subCallbacks: make(map[string]SubscribeCallback),
	}

	called := false
	cb := func(services []model.Instance, err error) {
		called = true
	}

	// Simulate callback storage (no real client needed)
	r.subMu.Lock()
	r.subCallbacks[subKey("test-svc", nil)] = cb
	r.subMu.Unlock()

	// Verify the callback was stored
	r.subMu.Lock()
	stored, ok := r.subCallbacks[subKey("test-svc", nil)]
	r.subMu.Unlock()

	if !ok {
		t.Fatal("expected callback to be stored")
	}
	stored(nil, nil)
	if !called {
		t.Error("expected stored callback to be callable")
	}
}
