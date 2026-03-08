package config

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	Name string `yaml:"name"`
	Env  string `yaml:"env"`
}

type LogConfig struct {
	Level        string `yaml:"level"`
	Format       string `yaml:"format"`
	Path         string `yaml:"path"`
	MaxSize      int    `yaml:"max_size"`
	MaxAge       int    `yaml:"max_age"`
	MaxBackups   int    `yaml:"max_backups"`
	Compress     bool   `yaml:"compress"`
	LogToConsole bool   `yaml:"log_to_console"`
}

type Config struct {
	App AppConfig `yaml:"app"`
	Log LogConfig `yaml:"log"`
}

type ConfigChangeCallback func(newCfg, oldCfg *Config)

var (
	globalConfig  *Config
	configMutex   sync.RWMutex
	callbacks     []ConfigChangeCallback
	callbackMutex sync.Mutex
	configPath    string
)

// 默认配置值
const (
	DefaultAppName       = "app"
	DefaultAppEnv        = "dev"
	DefaultLogLevel      = "info"
	DefaultLogFormat     = "console"
	DefaultLogPath       = "logs/app.log"
	DefaultLogMaxSize    = 200
	DefaultLogMaxAge     = 60
	DefaultLogMaxBackups = 60
	DefaultLogCompress   = true
	DefaultLogToConsole  = true
)

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

func Init(path string) error {
	configPath = path
	cfg, err := loadConfig(path)
	if err != nil {
		return err
	}

	configMutex.Lock()
	globalConfig = cfg
	configMutex.Unlock()

	return nil
}

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

func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

func AddWatch(callback ConfigChangeCallback) {
	callbackMutex.Lock()
	callbacks = append(callbacks, callback)
	callbackMutex.Unlock()
}

func triggerCallbacks(newCfg, oldCfg *Config) {
	callbackMutex.Lock()
	cbs := make([]ConfigChangeCallback, len(callbacks))
	copy(cbs, callbacks)
	callbackMutex.Unlock()

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

func updateConfig() error {
	newCfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}

	configMutex.Lock()
	oldCfg := globalConfig
	globalConfig = newCfg
	configMutex.Unlock()

	triggerCallbacks(newCfg, oldCfg)
	return nil
}

func CloseWatcher() {
	if watcher != nil {
		watcher.Close()
	}
}

// GenerateConfig 动态生成默认配置文件
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
func (c *Config) ToYAML() ([]byte, error) {
	return yaml.Marshal(c)
}
