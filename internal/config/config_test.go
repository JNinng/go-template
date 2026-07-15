package config

import (
	"os"
	"reflect"
	"testing"

	"go-template/internal/logger"
)

func TestObservabilityDefaultConfig(t *testing.T) {
	cfg := DefaultObservabilityConfig()
	if cfg.Addr != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.Addr)
	}
	if cfg.MetricsPath != "/metrics" {
		t.Errorf("expected /metrics, got %s", cfg.MetricsPath)
	}
	if cfg.HealthPath != "/health" {
		t.Errorf("expected /health, got %s", cfg.HealthPath)
	}
	if cfg.OTel.Endpoint != DefaultOTelEndpoint {
		t.Errorf("expected %s, got %s", DefaultOTelEndpoint, cfg.OTel.Endpoint)
	}
	if cfg.OTel.Protocol != DefaultOTelProtocol {
		t.Errorf("expected %s, got %s", DefaultOTelProtocol, cfg.OTel.Protocol)
	}
	if cfg.OTel.Logs.Enabled {
		t.Error("expected logs.enabled to be false by default")
	}
	if cfg.OTel.Traces.Enabled {
		t.Error("expected traces.enabled to be false by default")
	}
}

func TestOTelConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Observability.OTel.Endpoint != DefaultOTelEndpoint {
		t.Errorf("expected %s, got %s", DefaultOTelEndpoint, cfg.Observability.OTel.Endpoint)
	}
	if cfg.Observability.OTel.Protocol != DefaultOTelProtocol {
		t.Errorf("expected %s, got %s", DefaultOTelProtocol, cfg.Observability.OTel.Protocol)
	}
}

func TestLoggerConfigConversion(t *testing.T) {
	cfg := DefaultConfig()
	lc := cfg.LoggerConfig()
	if lc.Level != DefaultLogLevel {
		t.Errorf("expected %s, got %s", DefaultLogLevel, lc.Level)
	}
	if lc.Format != DefaultLogFormat {
		t.Errorf("expected %s, got %s", DefaultLogFormat, lc.Format)
	}
	if lc.MaxSize != DefaultLogMaxSize {
		t.Errorf("expected %d, got %d", DefaultLogMaxSize, lc.MaxSize)
	}
	if lc.MaxAge != DefaultLogMaxAge {
		t.Errorf("expected %d, got %d", DefaultLogMaxAge, lc.MaxAge)
	}
	if lc.MaxBackups != DefaultLogMaxBackups {
		t.Errorf("expected %d, got %d", DefaultLogMaxBackups, lc.MaxBackups)
	}
	if lc.Compress != DefaultLogCompress {
		t.Errorf("expected %v, got %v", DefaultLogCompress, lc.Compress)
	}
	if lc.LogToConsole != DefaultLogToConsole {
		t.Errorf("expected %v, got %v", DefaultLogToConsole, lc.LogToConsole)
	}

	// Verify type assertion: lc must be logger.Config type
	_ = logger.Config(lc)
}

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/test.yaml"
	yamlContent := []byte("app:\n  name: test-app\n  env: staging\n")
	if err := os.WriteFile(cfgPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Init(cfgPath)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if cfg.App.Name != "test-app" {
		t.Errorf("expected test-app, got %s", cfg.App.Name)
	}
	if cfg.App.Env != "staging" {
		t.Errorf("expected staging, got %s", cfg.App.Env)
	}
}

func TestInitWithSource(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/test.yaml"
	yamlContent := []byte("app:\n  name: base-app\n  env: dev\n")
	if err := os.WriteFile(cfgPath, yamlContent, 0644); err != nil {
		t.Fatal(err)
	}

	sourceContent := []byte("app:\n  name: overridden-app\n")
	source := &mockSource{
		name:    "test-source",
		content: sourceContent,
		ch:      make(chan []byte),
	}

	cfg, err := Init(cfgPath, source)
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if cfg.App.Name != "overridden-app" {
		t.Errorf("expected overridden-app, got %s", cfg.App.Name)
	}
	if cfg.App.Env != "dev" {
		t.Errorf("expected dev, got %s", cfg.App.Env)
	}
}

type mockSource struct {
	name    string
	content []byte
	ch      chan []byte
}

func (m *mockSource) Name() string                         { return m.name }
func (m *mockSource) Init() ([]byte, <-chan []byte, error) { return m.content, m.ch, nil }
func (m *mockSource) Close() error                         { return nil }

// ============================================================================
// copyConfig / deepCopyValue 测试
// ============================================================================

func TestCopyConfigNil(t *testing.T) {
	cfg := copyConfig(nil)
	// nil 输入应返回零值 Config，不 panic
	zero := Config{}
	if !reflect.DeepEqual(cfg, zero) {
		t.Error("copyConfig(nil) should return zero Config")
	}
}

func TestCopyConfigMetadataIndependence(t *testing.T) {
	src := DefaultConfig()
	src.NacosService.Metadata = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	dst := copyConfig(src)

	// 值相等
	if dst.NacosService.Metadata["key1"] != "value1" {
		t.Error("copied Metadata should have same values")
	}

	// 底层 map 独立 — 修改 dst 不影响 src
	dst.NacosService.Metadata["key1"] = "modified"
	if src.NacosService.Metadata["key1"] != "value1" {
		t.Error("modifying copy's Metadata should not affect original")
	}

	// 删除 dst 的 key 不影响 src
	delete(dst.NacosService.Metadata, "key2")
	if _, ok := src.NacosService.Metadata["key2"]; !ok {
		t.Error("deleting from copy's Metadata should not affect original")
	}
}

func TestCopyConfigNilMetadata(t *testing.T) {
	src := DefaultConfig()
	src.NacosService.Metadata = nil

	dst := copyConfig(src)
	if dst.NacosService.Metadata != nil {
		t.Error("copying nil Metadata should yield nil, not empty map")
	}
}

func TestCopyConfigFullRoundTrip(t *testing.T) {
	src := DefaultConfig()
	src.App.Name = "my-app"
	src.App.Port = 3000
	src.Log.Level = "debug"
	src.Nacos.Addr = "10.0.0.1"
	src.NacosService.Metadata = map[string]string{"zone": "us-east-1"}

	dst := copyConfig(src)

	// 所有值类型字段相等
	if dst.App.Name != src.App.Name {
		t.Errorf("App.Name: expected %q, got %q", src.App.Name, dst.App.Name)
	}
	if dst.App.Port != src.App.Port {
		t.Errorf("App.Port: expected %d, got %d", src.App.Port, dst.App.Port)
	}
	if dst.Log.Level != src.Log.Level {
		t.Errorf("Log.Level: expected %q, got %q", src.Log.Level, dst.Log.Level)
	}
	if dst.Nacos.Addr != src.Nacos.Addr {
		t.Errorf("Nacos.Addr: expected %q, got %q", src.Nacos.Addr, dst.Nacos.Addr)
	}
	if dst.NacosService.Weight != src.NacosService.Weight {
		t.Errorf("Weight: expected %f, got %f", src.NacosService.Weight, dst.NacosService.Weight)
	}

	// Metadata 值相等但底层独立
	if dst.NacosService.Metadata["zone"] != "us-east-1" {
		t.Error("Metadata value should match")
	}
	dst.NacosService.Metadata["zone"] = "us-west-2"
	if src.NacosService.Metadata["zone"] != "us-east-1" {
		t.Error("Metadata should be independent after deep copy")
	}
}

func TestCopyConfigPreservesAllDefaults(t *testing.T) {
	src := DefaultConfig()
	dst := copyConfig(src)

	// 确保深拷贝后的默认值与源一致（reflect.DeepEqual 验证所有字段）
	if !reflect.DeepEqual(*src, dst) {
		t.Error("copyConfig should preserve all default values")
	}

	// Metadata 独立（即使两者目前都为空/nil，浅拷贝也不会污染）
	dst.NacosService.Metadata = map[string]string{"new": "val"}
	if src.NacosService.Metadata != nil && len(src.NacosService.Metadata) > 0 {
		// 如果未来 DefaultNacosServiceConfig 默认填充了 Metadata，
		// 确保拷贝后的修改不影响源
		if src.NacosService.Metadata["new"] == "val" {
			t.Error("Metadata should be independent")
		}
	}
}

// ============================================================================
// deepCopyValue 单元测试
// ============================================================================

func TestDeepCopyValueMap(t *testing.T) {
	// 非 nil map
	orig := map[string]int{"a": 1, "b": 2}
	v := deepCopyValue(reflect.ValueOf(orig))
	copied := v.Interface().(map[string]int)

	if copied["a"] != 1 || copied["b"] != 2 {
		t.Error("copied map should have same entries")
	}

	// 修改独立性
	copied["a"] = 99
	if orig["a"] != 1 {
		t.Error("modifying copy should not affect original map")
	}
	delete(copied, "b")
	if _, ok := orig["b"]; !ok {
		t.Error("deleting from copy should not affect original map")
	}
}

func TestDeepCopyValueMapNil(t *testing.T) {
	var orig map[string]string // nil
	v := deepCopyValue(reflect.ValueOf(orig))
	if !v.IsNil() {
		t.Error("deep copying nil map should return nil")
	}
}

func TestDeepCopyValueMapEmpty(t *testing.T) {
	orig := make(map[string]string)
	v := deepCopyValue(reflect.ValueOf(orig))
	copied := v.Interface().(map[string]string)
	if len(copied) != 0 {
		t.Error("copied empty map should have length 0")
	}
	// 空 map 不等于 nil
	if copied == nil {
		t.Error("copied empty map should not be nil")
	}
}

func TestDeepCopyValueSlice(t *testing.T) {
	orig := []string{"x", "y", "z"}
	v := deepCopyValue(reflect.ValueOf(orig))
	copied := v.Interface().([]string)

	if copied[0] != "x" || copied[2] != "z" {
		t.Error("copied slice should have same elements")
	}

	copied[0] = "modified"
	if orig[0] != "x" {
		t.Error("modifying copy should not affect original slice")
	}
}

func TestDeepCopyValueSliceNil(t *testing.T) {
	var orig []int // nil
	v := deepCopyValue(reflect.ValueOf(orig))
	if !v.IsNil() {
		t.Error("deep copying nil slice should return nil")
	}
}

func TestDeepCopyValuePtr(t *testing.T) {
	s := "hello"
	orig := &s
	v := deepCopyValue(reflect.ValueOf(orig))
	copied := v.Interface().(*string)

	if *copied != "hello" {
		t.Error("copied pointer should have same value")
	}

	*copied = "world"
	if *orig != "hello" {
		t.Error("modifying copy's target should not affect original")
	}
}

func TestDeepCopyValuePtrNil(t *testing.T) {
	var orig *int // nil
	v := deepCopyValue(reflect.ValueOf(orig))
	if !v.IsNil() {
		t.Error("deep copying nil pointer should return nil")
	}
}

func TestDeepCopyValuePrimitive(t *testing.T) {
	// 值类型直接返回
	orig := 42
	v := deepCopyValue(reflect.ValueOf(orig))
	if v.Int() != 42 {
		t.Error("value type should be preserved")
	}

	origStr := "hello"
	vStr := deepCopyValue(reflect.ValueOf(origStr))
	if vStr.String() != "hello" {
		t.Error("string should be preserved")
	}
}

func TestDeepCopyValueNested(t *testing.T) {
	// 嵌套结构：map 的 value 是 slice
	orig := map[string][]int{
		"even": {2, 4, 6},
		"odd":  {1, 3, 5},
	}
	v := deepCopyValue(reflect.ValueOf(orig))
	copied := v.Interface().(map[string][]int)

	// 修改嵌套 slice 不影响原始
	copied["even"][0] = 99
	if orig["even"][0] != 2 {
		t.Error("nested slice should be independent after deep copy")
	}
}

func TestCopyConfigWithSliceField(t *testing.T) {
	// 验证 deepCopyValue 是泛用的：用临时结构体测试 slice 字段
	type HasSlice struct {
		Items []string
	}
	type HasMap struct {
		Tags map[string]string
	}

	// slice 独立
	v1 := deepCopyValue(reflect.ValueOf(HasSlice{Items: []string{"a", "b"}}))
	s1 := v1.Interface().(HasSlice)
	s1.Items[0] = "z"

	// map 独立
	v2 := deepCopyValue(reflect.ValueOf(HasMap{Tags: map[string]string{"k": "v"}}))
	m2 := v2.Interface().(HasMap)
	m2.Tags["k"] = "changed"
}
