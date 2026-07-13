package nacos

import (
	"testing"

	"go-template/internal/config"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
)

func TestNewRegistrar(t *testing.T) {
	cfg := config.DefaultNacosServiceConfig()
	nc := config.DefaultNacosConfig()
	ac := config.DefaultAppConfig()
	cfg.ApplyDefaults(&nc, &ac)

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

	// 不可继承字段 — 应有默认值
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

	// 可继承字段 — 应为零值，等待 ApplyDefaults 填充
	if cfg.ServiceName != "" {
		t.Errorf("expected ServiceName empty before ApplyDefaults, got %q", cfg.ServiceName)
	}
	if cfg.ServicePort != 0 {
		t.Errorf("expected ServicePort 0 before ApplyDefaults, got %d", cfg.ServicePort)
	}
	if cfg.Addr != "" {
		t.Errorf("expected Addr empty before ApplyDefaults, got %q", cfg.Addr)
	}

	// ApplyDefaults 后应有回退值
	nc := config.DefaultNacosConfig()
	ac := config.DefaultAppConfig()
	cfg.ApplyDefaults(&nc, &ac)

	if cfg.ServiceName != ac.Name {
		t.Errorf("expected ServiceName %q after ApplyDefaults, got %q", ac.Name, cfg.ServiceName)
	}
	if cfg.ServicePort != uint64(ac.Port) {
		t.Errorf("expected ServicePort %d after ApplyDefaults, got %d", ac.Port, cfg.ServicePort)
	}
	if cfg.Addr != nc.Addr {
		t.Errorf("expected Addr %q after ApplyDefaults, got %q", nc.Addr, cfg.Addr)
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

	// 可继承字段应为零值
	if ns.Addr != "" {
		t.Errorf("expected Addr empty, got %q", ns.Addr)
	}
	if ns.Port != 0 {
		t.Errorf("expected Port 0, got %d", ns.Port)
	}
	if ns.Group != "" {
		t.Errorf("expected Group empty, got %q", ns.Group)
	}

	// ApplyDefaults 后应有回退值
	nc := config.DefaultNacosConfig()
	ac := config.DefaultAppConfig()
	ns.ApplyDefaults(&nc, &ac)

	if ns.Addr != nc.Addr {
		t.Errorf("expected Addr %q after ApplyDefaults, got %q", nc.Addr, ns.Addr)
	}
	if ns.Port != nc.Port {
		t.Errorf("expected Port %d after ApplyDefaults, got %d", nc.Port, ns.Port)
	}
	if ns.Group != nc.Group {
		t.Errorf("expected Group %q after ApplyDefaults, got %q", nc.Group, ns.Group)
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
