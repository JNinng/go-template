// Package config 提供配置管理功能
//
// 功能特点:
//   - 支持从 YAML 文件加载配置 (基于 Viper)
//   - 配置热更新 (通过 Viper WatchConfig)
//   - 线程安全的配置读取
//   - 支持配置变更监听
//   - 支持环境变量覆盖 (前缀 APP_, 如 APP_LOG_LEVEL=debug)
package config

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"go-template/internal/logger"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// AppConfig 应用程序基础配置
type AppConfig struct {
	Name  string `yaml:"name" mapstructure:"name"`   // 应用名称
	Env   string `yaml:"env" mapstructure:"env"`     // 运行环境
	Port  int    `yaml:"port" mapstructure:"port"`   // 应用端口
	Watch bool   `yaml:"watch" mapstructure:"watch"` // 是否监控配置文件变更
}

// LogConfig 日志配置
type LogConfig struct {
	Level        string `yaml:"level" mapstructure:"level"`                   // 日志级别
	Format       string `yaml:"format" mapstructure:"format"`                 // 日志格式 (console/json)
	Path         string `yaml:"path" mapstructure:"path"`                     // 日志文件路径
	MaxSize      int    `yaml:"max_size" mapstructure:"max_size"`             // 单个日志文件最大大小 (MB)
	MaxAge       int    `yaml:"max_age" mapstructure:"max_age"`               // 日志文件保留天数
	MaxBackups   int    `yaml:"max_backups" mapstructure:"max_backups"`       // 保留的日志文件数量
	Compress     bool   `yaml:"compress" mapstructure:"compress"`             // 是否压缩历史日志
	LogToConsole bool   `yaml:"log_to_console" mapstructure:"log_to_console"` // 是否输出到控制台
}

// NacosConfig Nacos 配置中心配置
type NacosConfig struct {
	Enabled   bool   `yaml:"enabled"   mapstructure:"enabled"`
	Addr      string `yaml:"addr"      mapstructure:"addr"`
	Port      uint64 `yaml:"port"      mapstructure:"port"`
	Username  string `yaml:"username"  mapstructure:"username"`
	Password  string `yaml:"password"  mapstructure:"password"`
	Namespace string `yaml:"namespace" mapstructure:"namespace"`
	Group     string `yaml:"group"     mapstructure:"group"`
	DataId    string `yaml:"data_id"   mapstructure:"data_id"`
	LogLevel  string `yaml:"log_level" mapstructure:"log_level"`
	LogDir    string `yaml:"log_dir"   mapstructure:"log_dir"`
	CacheDir  string `yaml:"cache_dir" mapstructure:"cache_dir"`
}

// NacosServiceConfig Nacos 服务注册与发现配置
type NacosServiceConfig struct {
	Enabled     bool              `yaml:"enabled"      mapstructure:"enabled"`
	Addr        string            `yaml:"addr"         mapstructure:"addr"`
	Port        uint64            `yaml:"port"         mapstructure:"port"`
	Namespace   string            `yaml:"namespace"    mapstructure:"namespace"`
	Group       string            `yaml:"group"        mapstructure:"group"`
	Username    string            `yaml:"username"     mapstructure:"username"`
	Password    string            `yaml:"password"     mapstructure:"password"`
	ServiceName string            `yaml:"service_name" mapstructure:"service_name"`
	ClusterName string            `yaml:"cluster_name" mapstructure:"cluster_name"`
	ServiceIP   string            `yaml:"service_ip"   mapstructure:"service_ip"`
	ServicePort uint64            `yaml:"service_port" mapstructure:"service_port"`
	Weight      float64           `yaml:"weight"       mapstructure:"weight"`
	Healthy     bool              `yaml:"healthy"      mapstructure:"healthy"`
	Metadata    map[string]string `yaml:"metadata"     mapstructure:"metadata"`
	LogLevel    string            `yaml:"log_level"    mapstructure:"log_level"`
	LogDir      string            `yaml:"log_dir"      mapstructure:"log_dir"`
	CacheDir    string            `yaml:"cache_dir"    mapstructure:"cache_dir"`
}

// ObservabilityConfig 可观测性配置
type ObservabilityConfig struct {
	Enabled     bool       `yaml:"enabled" mapstructure:"enabled"`
	SinglePort  bool       `yaml:"single_port" mapstructure:"single_port"` // true: handler 注入业务 server；false: 独立 HTTP server
	Addr        string     `yaml:"addr" mapstructure:"addr"`
	MetricsPath string     `yaml:"metrics_path" mapstructure:"metrics_path"`
	HealthPath  string     `yaml:"health_path" mapstructure:"health_path"`
	OTel        OTelConfig `yaml:"otel" mapstructure:"otel"`
}

// OTelConfig OpenTelemetry 配置
type OTelConfig struct {
	Enabled  bool         `yaml:"enabled" mapstructure:"enabled"`   // 是否启用 OpenTelemetry（总开关）
	Endpoint string       `yaml:"endpoint" mapstructure:"endpoint"` // OTLP collector 地址
	Protocol string       `yaml:"protocol" mapstructure:"protocol"` // 协议: "grpc" 或 "http"
	Logs     SignalConfig `yaml:"logs" mapstructure:"logs"`         // 日志导出配置
	Traces   SignalConfig `yaml:"traces" mapstructure:"traces"`     // 链路导出配置
}

// SignalConfig 单个 OTel 信号的启用配置
type SignalConfig struct {
	Enabled bool `yaml:"enabled" mapstructure:"enabled"`
}

// Config 完整配置结构
type Config struct {
	App           AppConfig           `yaml:"app" mapstructure:"app"`
	Log           LogConfig           `yaml:"log" mapstructure:"log"`
	Nacos         NacosConfig         `yaml:"nacos_config" mapstructure:"nacos_config"`
	NacosService  NacosServiceConfig  `yaml:"nacos_service" mapstructure:"nacos_service"`
	Observability ObservabilityConfig `yaml:"observability" mapstructure:"observability"`
	Secret        struct {
		Key string `yaml:"key" mapstructure:"key"`
	} `yaml:"secret" mapstructure:"secret"`
}

// ChangeCallback 配置变更回调函数
// newCfg: 新的配置对象
// oldCfg: 旧的配置对象
type ChangeCallback func(newCfg, oldCfg *Config)

// WatchKey 监听器唯一标识符
// 通过 AddWatch 返回，用于取消监听
type WatchKey int

var (
	globalConfig    atomic.Pointer[Config]      // 全局配置指针 (原子操作保证线程安全)
	callbacks       map[WatchKey]ChangeCallback // 配置变更回调函数映射
	sourcesMu       sync.Mutex                  // 保护 sources 切片的互斥锁
	sources         []Source                    // 已初始化的外部配置源 (用于 shutdown 清理)
	callbackRWMutex sync.RWMutex                // 回调函数表的读写锁
	nextWatchKey    WatchKey                    // 下一个可用的 WatchKey
	v               *viper.Viper                // Viper 实例
)

// 默认配置值
const (
	DefaultAppName            = "app"          // 默认应用名称
	DefaultAppEnv             = "dev"          // 默认运行环境
	DefaultAppPort            = 8080           // 默认应用端口
	DefaultAppWatch           = false          // 默认监控配置文件变更
	DefaultLogLevel           = "info"         // 默认日志级别
	DefaultLogFormat          = "console"      // 默认日志格式
	DefaultLogPath            = "logs/app.log" // 默认日志路径
	DefaultLogMaxSize         = 200            // 默认单个日志文件最大大小 (MB)
	DefaultLogMaxAge          = 60             // 默认日志文件保留天数
	DefaultLogMaxBackups      = 60             // 默认保留的日志文件数量
	DefaultLogCompress        = true           // 默认启用日志压缩
	DefaultLogToConsole       = true           // 默认启用控制台输出
	DefaultObsAddr            = ":9090"
	DefaultObsMetricsPath     = "/metrics"
	DefaultObsHealthPath      = "/health"
	DefaultNacosServiceWeight = 10
	DefaultOTelEnabled        = false
	DefaultOTelEndpoint       = ""
	DefaultOTelProtocol       = "grpc"
)

// DefaultAppConfig 返回默认应用配置
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Name:  DefaultAppName,
		Env:   DefaultAppEnv,
		Port:  DefaultAppPort,
		Watch: DefaultAppWatch,
	}
}

// DefaultLogConfig 返回默认日志配置
func DefaultLogConfig() LogConfig {
	return LogConfig{
		Level:        DefaultLogLevel,
		Format:       DefaultLogFormat,
		Path:         DefaultLogPath,
		MaxSize:      DefaultLogMaxSize,
		MaxAge:       DefaultLogMaxAge,
		MaxBackups:   DefaultLogMaxBackups,
		Compress:     DefaultLogCompress,
		LogToConsole: DefaultLogToConsole,
	}
}

// DefaultObservabilityConfig 返回默认可观测性配置
func DefaultObservabilityConfig() ObservabilityConfig {
	return ObservabilityConfig{
		SinglePort:  false,
		Addr:        DefaultObsAddr,
		MetricsPath: DefaultObsMetricsPath,
		HealthPath:  DefaultObsHealthPath,
		OTel: OTelConfig{
			Enabled:  DefaultOTelEnabled,
			Endpoint: DefaultOTelEndpoint,
			Protocol: DefaultOTelProtocol,
		},
	}
}

// DefaultNacosConfig 返回默认 Nacos 配置
func DefaultNacosConfig() NacosConfig {
	return NacosConfig{
		Enabled:   false,
		Addr:      "127.0.0.1",
		Port:      8848,
		Username:  "nacos",
		Password:  "nacos",
		Namespace: "public",
		Group:     "DEFAULT_GROUP",
		DataId:    "application.yml",
		LogLevel:  "debug",
		LogDir:    "./logs",
		CacheDir:  "./cache",
	}
}

// DefaultNacosServiceConfig 返回默认 Nacos 服务注册配置
//
// 所有连接参数自带独立默认值，不再从 NacosConfig 继承。
// ServiceName 和 ServicePort 需业务显式配置，无默认值。
func DefaultNacosServiceConfig() NacosServiceConfig {
	return NacosServiceConfig{
		Enabled:     false,
		Addr:        "127.0.0.1",
		Port:        8848,
		Username:    "nacos",
		Password:    "nacos",
		Namespace:   "public",
		Group:       "DEFAULT_GROUP",
		ClusterName: "DEFAULT",
		ServiceIP:   "127.0.0.1",
		Weight:      DefaultNacosServiceWeight,
		Healthy:     true,
		LogLevel:    "debug",
		LogDir:      "./logs",
		CacheDir:    "./cache",
	}
}

// DefaultConfig 返回默认配置
//
// 返回值:
//   - *Config: 包含所有默认值的配置对象
func DefaultConfig() *Config {
	return &Config{
		App:           DefaultAppConfig(),
		Log:           DefaultLogConfig(),
		Nacos:         DefaultNacosConfig(),
		NacosService:  DefaultNacosServiceConfig(),
		Observability: DefaultObservabilityConfig(),
	}
}

// Init 初始化配置系统
//
// 参数:
//   - path: 配置文件路径
//   - sources: 可选的外部配置源列表
//
// 返回值:
//   - *Config: 初始化的配置对象
//   - error: 加载配置失败时返回错误
func Init(path string, sources ...Source) (*Config, error) {
	callbacks = make(map[WatchKey]ChangeCallback)

	v = viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	setDefaults()

	v.SetEnvPrefix("APP")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := v.Unmarshal(cfg); err != nil {
		return nil, err
	}

	// 从外部配置源合并
	type pendingWatcher struct {
		name    string
		changes <-chan []byte
	}
	var pending []pendingWatcher

	for _, s := range sources {
		content, changes, err := initSource(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: source %s init failed: %v\n", s.Name(), err)
			continue
		}
		if content != nil {
			if err := yaml.Unmarshal(content, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: source %s content unmarshal failed: %v\n", s.Name(), err)
			}
		}
		if changes != nil {
			pending = append(pending, pendingWatcher{s.Name(), changes})
		}
	}

	if cfg.App.Watch {
		StartWatcher()
	}
	// 先存储合并后的配置，再启动 watch goroutine
	// 避免远程变更在 Store 之前到达时被覆盖丢失
	globalConfig.Store(cfg)

	for _, pw := range pending {
		go watchSource(pw.name, pw.changes)
	}

	return cfg, nil
}

// initSource 初始化单个外部配置源并追踪其生命周期
//
// 参数:
//   - s: 外部配置源
//
// 返回值:
//   - content: 初始配置内容 (nil if empty)
//   - changes: 配置变更通道 (nil if not supported)
//   - error: 初始化失败时返回错误
func initSource(s Source) (content []byte, changes <-chan []byte, err error) {
	content, changes, err = s.Init()
	if err != nil {
		return nil, nil, err
	}

	sourcesMu.Lock()
	sources = append(sources, s)
	sourcesMu.Unlock()
	return content, changes, nil
}

// MergeSource 在 Init 之后合并外部配置源的内容
//
// 用于两阶段初始化场景：先用本地配置获取连接参数，再连接远程配置中心。
// Nacos 的 addr/port/namespace 等从本地配置文件读取，业务配置从远程获取。
//
// 参数:
//   - source: 外部配置源
//
// 返回值:
//   - error: 合并失败时返回错误
func MergeSource(source Source) error {
	content, changes, err := initSource(source)
	if err != nil {
		return err
	}

	oldCfg := globalConfig.Load()
	if oldCfg == nil {
		return fmt.Errorf("config not initialized, call Init first")
	}

	newCfg := copyConfig(oldCfg)
	if content != nil {
		if err := yaml.Unmarshal(content, &newCfg); err != nil {
			return err
		}
	}

	if !reflect.DeepEqual(*oldCfg, newCfg) {
		triggerCallbacks(&newCfg, oldCfg)
		globalConfig.Store(&newCfg)
	}

	// 在配置存储之后再启动 watch goroutine，避免远程变更
	// 在 MergeSource 完成 Store 之前到达时被覆盖丢失
	if changes != nil {
		go watchSource(source.Name(), changes)
	}

	return nil
}

// copyConfig 深拷贝 Config，确保 map/slice/ptr 不共享底层引用
//
// 使用反射递归遍历所有字段：map 创建新 map 并拷贝每个 entry，
// slice 创建新 slice 并拷贝每个元素，指针分配新内存并拷贝指向的值，
// 结构体递归处理每个导出字段，值类型（string/int/bool/...）直接复制。
// 这确保 unmarshal 修改新配置时不会污染旧配置，DeepEqual 比较正确工作，
// 且未来新增任何 map/slice 字段自动获得深拷贝保护。
func copyConfig(src *Config) Config {
	if src == nil {
		return Config{}
	}
	return deepCopyValue(reflect.ValueOf(src).Elem()).Interface().(Config)
}

// deepCopyValue 递归深拷贝任意值
//
// map/slice/ptr 类型创建新的底层存储并递归拷贝元素，
// 结构体递归处理每个导出字段，值类型直接返回（Go 中值类型赋值即拷贝）。
func deepCopyValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		m := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			m.SetMapIndex(iter.Key(), deepCopyValue(iter.Value()))
		}
		return m
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		s := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		for i := 0; i < v.Len(); i++ {
			s.Index(i).Set(deepCopyValue(v.Index(i)))
		}
		return s
	case reflect.Ptr:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		p := reflect.New(v.Type().Elem())
		p.Elem().Set(deepCopyValue(v.Elem()))
		return p
	case reflect.Struct:
		s := reflect.New(v.Type()).Elem()
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				s.Field(i).Set(deepCopyValue(v.Field(i)))
			}
		}
		return s
	default:
		// string, int, bool, float 等值类型 — 赋值即拷贝
		return v
	}
}

// CloseSources 关闭所有已注册的外部配置源并取消监听
//
// 应在应用 shutdown 时调用，确保所有远程配置源的连接和监听器被正确释放。
// 每个源的 Close 错误会被记录到 stderr，但不会中断其他源的关闭。
func CloseSources() {
	sourcesMu.Lock()
	defer sourcesMu.Unlock()

	for _, s := range sources {
		if err := s.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing config source %s: %v\n", s.Name(), err)
		}
	}
	sources = nil
}

// watchSource 监听外部配置源的变更
func watchSource(name string, changes <-chan []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "Panic in source watcher %s: %v\n", name, r)
		}
	}()
	for data := range changes {
		updateConfig(data)
	}
}

func updateConfig(data []byte) {
	oldCfg := globalConfig.Load()
	if oldCfg == nil {
		return
	}

	newCfg := copyConfig(oldCfg)
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		return
	}

	if !reflect.DeepEqual(oldCfg, &newCfg) {
		triggerCallbacks(&newCfg, oldCfg)
		globalConfig.Store(&newCfg)
	}
}

// setDefaults 设置 Viper 默认值
func setDefaults() {
	app := DefaultAppConfig()
	v.SetDefault("app.name", app.Name)
	v.SetDefault("app.env", app.Env)
	v.SetDefault("app.port", app.Port)
	v.SetDefault("app.watch", app.Watch)

	log := DefaultLogConfig()
	v.SetDefault("log.level", log.Level)
	v.SetDefault("log.format", log.Format)
	v.SetDefault("log.path", log.Path)
	v.SetDefault("log.max_size", log.MaxSize)
	v.SetDefault("log.max_age", log.MaxAge)
	v.SetDefault("log.max_backups", log.MaxBackups)
	v.SetDefault("log.compress", log.Compress)
	v.SetDefault("log.log_to_console", log.LogToConsole)

	obs := DefaultObservabilityConfig()
	v.SetDefault("observability.single_port", obs.SinglePort)
	v.SetDefault("observability.addr", obs.Addr)
	v.SetDefault("observability.metrics_path", obs.MetricsPath)
	v.SetDefault("observability.health_path", obs.HealthPath)
	v.SetDefault("observability.otel.enabled", obs.OTel.Enabled)
	v.SetDefault("observability.otel.endpoint", obs.OTel.Endpoint)
	v.SetDefault("observability.otel.protocol", obs.OTel.Protocol)
	v.SetDefault("observability.otel.logs.enabled", obs.OTel.Logs.Enabled)
	v.SetDefault("observability.otel.traces.enabled", obs.OTel.Traces.Enabled)

	nc := DefaultNacosConfig()
	v.SetDefault("nacos_config.enabled", nc.Enabled)
	v.SetDefault("nacos_config.addr", nc.Addr)
	v.SetDefault("nacos_config.port", nc.Port)
	v.SetDefault("nacos_config.username", nc.Username)
	v.SetDefault("nacos_config.password", nc.Password)
	v.SetDefault("nacos_config.namespace", nc.Namespace)
	v.SetDefault("nacos_config.group", nc.Group)
	v.SetDefault("nacos_config.data_id", nc.DataId)
	v.SetDefault("nacos_config.log_level", nc.LogLevel)
	v.SetDefault("nacos_config.log_dir", nc.LogDir)
	v.SetDefault("nacos_config.cache_dir", nc.CacheDir)

	ns := DefaultNacosServiceConfig()
	v.SetDefault("nacos_service.enabled", ns.Enabled)
	v.SetDefault("nacos_service.addr", ns.Addr)
	v.SetDefault("nacos_service.port", ns.Port)
	v.SetDefault("nacos_service.username", ns.Username)
	v.SetDefault("nacos_service.password", ns.Password)
	v.SetDefault("nacos_service.namespace", ns.Namespace)
	v.SetDefault("nacos_service.group", ns.Group)
	v.SetDefault("nacos_service.cluster_name", ns.ClusterName)
	v.SetDefault("nacos_service.service_ip", ns.ServiceIP)
	v.SetDefault("nacos_service.weight", ns.Weight)
	v.SetDefault("nacos_service.healthy", ns.Healthy)
	v.SetDefault("nacos_service.log_level", ns.LogLevel)
	v.SetDefault("nacos_service.log_dir", ns.LogDir)
	v.SetDefault("nacos_service.cache_dir", ns.CacheDir)
}

// Get 获取当前配置
//
// 返回值:
//   - *Config: 当前配置对象的指针
func Get() *Config {
	return globalConfig.Load()
}

// AddWatch 注册配置变更监听器
//
// 参数:
//   - callback: 配置变更时的回调函数
//
// 返回值:
//   - WatchKey: 监听器唯一标识，用于取消监听
func AddWatch(callback ChangeCallback) WatchKey {
	callbackRWMutex.Lock()
	defer callbackRWMutex.Unlock()

	key := nextWatchKey
	nextWatchKey++
	callbacks[key] = callback
	return key
}

// RemoveWatch 取消配置变更监听
//
// 参数:
//   - key: AddWatch 返回的监听器标识
func RemoveWatch(key WatchKey) {
	callbackRWMutex.Lock()
	defer callbackRWMutex.Unlock()
	delete(callbacks, key)
}

// triggerCallbacks 触发所有配置变更回调
// 在持有读锁的情况下复制回调列表，然后在无锁状态下异步执行
//
// 参数:
//   - newCfg: 新的配置对象
//   - oldCfg: 旧的配置对象
func triggerCallbacks(newCfg, oldCfg *Config) {
	callbackRWMutex.RLock()
	cbs := make([]ChangeCallback, 0, len(callbacks))
	for _, cb := range callbacks {
		cbs = append(cbs, cb)
	}
	callbackRWMutex.RUnlock()

	for _, cb := range cbs {
		go func(callback ChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
				}
			}()
			callback(newCfg, oldCfg)
		}(cb)
	}
}

// reloadConfig 从 Viper 当前状态重新解析配置并触发回调
//
// 返回值:
//   - error: 解析失败时返回错误
func reloadConfig() error {
	oldCfg := globalConfig.Load()
	if oldCfg == nil {
		return nil // 配置尚未初始化，忽略此次事件
	}
	newCfg := copyConfig(oldCfg)
	if err := v.Unmarshal(&newCfg); err != nil {
		return err
	}

	if reflect.DeepEqual(oldCfg, &newCfg) {
		return nil
	}

	triggerCallbacks(&newCfg, oldCfg)
	globalConfig.Store(&newCfg)
	return nil
}

// GenerateConfig 生成默认配置文件
//
// 参数:
//   - outputPath: 输出文件路径，为空时使用默认路径 "config.yaml"
func GenerateConfig(outputPath string) {
	if outputPath == "" {
		outputPath = "config.yaml"
	}

	cfg := DefaultConfig()
	data, err := cfg.ToYAML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal config: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to generate config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config file generated: %s\n", outputPath)
}

// ToYAML 将配置转换为 YAML 格式
//
// 返回值:
//   - []byte: YAML 格式的字节数据
//   - error: 转换失败时返回错误
func (c *Config) ToYAML() ([]byte, error) {
	return yaml.Marshal(c)
}

// LoggerConfig 返回 logger.Config，用于初始化日志系统
func (c *Config) LoggerConfig() logger.Config {
	return logger.Config{
		Level:        c.Log.Level,
		Format:       c.Log.Format,
		Path:         c.Log.Path,
		MaxSize:      c.Log.MaxSize,
		MaxAge:       c.Log.MaxAge,
		MaxBackups:   c.Log.MaxBackups,
		Compress:     c.Log.Compress,
		LogToConsole: c.Log.LogToConsole,
	}
}

var once sync.Once

// StartWatcher 启动配置文件监控
// 使用 Viper 内置的 WatchConfig 机制监听配置文件变更
func StartWatcher() {
	if v == nil {
		return
	}
	once.Do(func() {
		v.OnConfigChange(func(_ fsnotify.Event) {
			if err := reloadConfig(); err != nil {
				fmt.Fprintf(os.Stderr, "Error reloading config: %v\n", err)
			}
		})
		v.WatchConfig()
	})
}
