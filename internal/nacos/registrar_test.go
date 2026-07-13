package nacos

import (
	"testing"

	"go-template/internal/config"

	"github.com/nacos-group/nacos-sdk-go/v2/model"
)

func TestNewRegistrar(t *testing.T) {
	cfg := config.DefaultNacosServiceConfig()
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
	if cfg.ServiceName != config.DefaultAppName {
		t.Errorf("expected ServiceName %q, got %q", config.DefaultAppName, cfg.ServiceName)
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
	if cfg.ServicePort != 8080 {
		t.Errorf("expected ServicePort 8080, got %d", cfg.ServicePort)
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
	if ns.Addr != "127.0.0.1" {
		t.Errorf("expected Addr 127.0.0.1, got %q", ns.Addr)
	}
	if ns.Port != 8848 {
		t.Errorf("expected Port 8848, got %d", ns.Port)
	}
	if ns.Group != "DEFAULT_GROUP" {
		t.Errorf("expected Group DEFAULT_GROUP, got %q", ns.Group)
	}
}
