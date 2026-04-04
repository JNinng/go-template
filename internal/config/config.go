// Package config 提供配置管理功能
//
// 功能特点:
//   - 支持从 YAML 文件加载配置
//   - 配置热更新 (通过文件监视)
//   - 线程安全的配置读取
//   - 支持配置变更监听
package config

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// AppConfig 应用程序基础配置
type AppConfig struct {
	Name string `yaml:"name"` // 应用名称
	Env  string `yaml:"env"`  // 运行环境
}

// LogConfig 日志配置
type LogConfig struct {
	Level        string `yaml:"level"`          // 日志级别
	Format       string `yaml:"format"`         // 日志格式 (console/json)
	Path         string `yaml:"path"`           // 日志文件路径
	MaxSize      int    `yaml:"max_size"`       // 单个日志文件最大大小 (MB)
	MaxAge       int    `yaml:"max_age"`        // 日志文件保留天数
	MaxBackups   int    `yaml:"max_backups"`    // 保留的日志文件数量
	Compress     bool   `yaml:"compress"`       // 是否压缩历史日志
	LogToConsole bool   `yaml:"log_to_console"` // 是否输出到控制台
}

// Config 完整配置结构
type Config struct {
	App AppConfig `yaml:"app"` // 应用配置
	Log LogConfig `yaml:"log"` // 日志配置
}

// ConfigChangeCallback 配置变更回调函数
// newCfg: 新的配置对象
// oldCfg: 旧的配置对象
type ConfigChangeCallback func(newCfg, oldCfg *Config)

// WatchKey 监听器唯一标识符
// 通过 AddWatch 返回，用于取消监听
type WatchKey int

var (
	globalConfig    atomic.Pointer[Config]            // 全局配置指针 (原子操作保证线程安全)
	callbacks       map[WatchKey]ConfigChangeCallback // 配置变更回调函数映射
	callbackRWMutex sync.RWMutex                      // 回调函数表的读写锁
	nextWatchKey    WatchKey                          // 下一个可用的 WatchKey
	configPath      string                            // 配置文件路径
)

// 默认配置值
const (
	DefaultAppName       = "app"          // 默认应用名称
	DefaultAppEnv        = "dev"          // 默认运行环境
	DefaultLogLevel      = "info"         // 默认日志级别
	DefaultLogFormat     = "console"      // 默认日志格式
	DefaultLogPath       = "logs/app.log" // 默认日志路径
	DefaultLogMaxSize    = 200            // 默认单个日志文件最大大小 (MB)
	DefaultLogMaxAge     = 60             // 默认日志文件保留天数
	DefaultLogMaxBackups = 60             // 默认保留的日志文件数量
	DefaultLogCompress   = true           // 默认启用日志压缩
	DefaultLogToConsole  = true           // 默认启用控制台输出
)

// DefaultConfig 返回默认配置
//
// 返回值:
//   - *Config: 包含所有默认值的配置对象
func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			Name: DefaultAppName,
			Env:  DefaultAppEnv,
		},
		Log: LogConfig{
			Level:        DefaultLogLevel,
			Format:       DefaultLogFormat,
			Path:         DefaultLogPath,
			MaxSize:      DefaultLogMaxSize,
			MaxAge:       DefaultLogMaxAge,
			MaxBackups:   DefaultLogMaxBackups,
			Compress:     DefaultLogCompress,
			LogToConsole: DefaultLogToConsole,
		},
	}
}

// Init 初始化配置系统
//
// 参数:
//   - path: 配置文件路径
//
// 返回值:
//   - error: 加载配置失败时返回错误
func Init(path string) error {
	configPath = path
	callbacks = make(map[WatchKey]ConfigChangeCallback)

	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	globalConfig.Store(cfg)
	return nil
}

// loadConfig 从指定路径加载配置文件
//
// 参数:
//   - path: 配置文件路径
//
// 返回值:
//   - *Config: 加载后的配置对象
//   - error: 加载失败时返回错误
func loadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
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
func AddWatch(callback ConfigChangeCallback) WatchKey {
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
	cbs := make([]ConfigChangeCallback, 0, len(callbacks))
	for _, cb := range callbacks {
		cbs = append(cbs, cb)
	}
	callbackRWMutex.RUnlock()

	for _, cb := range cbs {
		go func(callback ConfigChangeCallback) {
			defer func() {
				if r := recover(); r != nil {
				}
			}()
			callback(newCfg, oldCfg)
		}(cb)
	}
}

// updateConfig 重新加载配置文件并触发回调
//
// 返回值:
//   - error: 加载失败时返回错误
func updateConfig() error {
	newCfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	oldCfg := globalConfig.Swap(newCfg)
	if reflect.DeepEqual(newCfg, oldCfg) {
		return nil
	}

	triggerCallbacks(newCfg, oldCfg)
	return nil
}

// GenerateConfig 生成默认配置文件
//
// 参数:
//   - outputPath: 输出文件路径，为空时使用默认路径 "config.yaml"
func GenerateConfig(outputPath string) {
	// 如果未指定输出路径，使用默认路径
	if outputPath == "" {
		outputPath = "config.yaml"
	}

	// 使用配置包的默认配置生成 YAML
	cfg := DefaultConfig()
	data, err := cfg.ToYAML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal config: %v\n", err)
		os.Exit(1)
	}

	// 写入配置文件
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
