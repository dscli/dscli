package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Config 配置管理器
type Config struct {
	mu        sync.RWMutex
	data      map[string]any
	configDir string
}

// New 创建新的配置管理器
func New() (*Config, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	cfg := &Config{
		configDir: configDir,
		data:      make(map[string]any),
	}

	if err := cfg.load(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}

// NewWithDir 使用指定目录创建配置管理器
func NewWithDir(dir string) (*Config, error) {
	cfg := &Config{
		configDir: dir,
		data:      make(map[string]any),
	}

	if err := cfg.load(); err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return cfg, nil
}

// Get 获取配置值
func (c *Config) Get(name, defaultValue string, alias ...string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if value, ok := c.data[name]; ok && value != "" {
		return fmt.Sprint(value)
	}

	for _, aliasName := range alias {
		if value, ok := c.data[aliasName]; ok && value != "" {
			return fmt.Sprint(value)
		}
	}

	return defaultValue
}

// Set 设置配置值（仅内存中）
func (c *Config) Set(name, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[name] = value
}

// GetValue 获取配置值，返回原始类型（支持 map、slice 等结构化数据）。
// 未配置时返回 nil。
func (c *Config) GetValue(name string) any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[name]
}

// SetValue 设置配置值，支持结构化数据（map、slice 等）。
// 仅在内存中设置，不持久化。调用 Save() 写入文件。
func (c *Config) SetValue(name string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[name] = value
}


// Save 保存配置到文件
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return saveConfigToFile(c.configDir, c.data)
}

// ConfigDir 返回配置目录
func (c *Config) ConfigDir() string {
	return c.configDir
}

// load 加载配置
func (c *Config) load() error {
	// 尝试从新格式文件加载
	configFile := filepath.Join(c.configDir, "config.dscli")
	defer c.Set("filename", configFile)
	if data, err := ParseFile(configFile); err == nil && len(data) > 0 {
		c.data = data
		return nil
	}

	// 从环境变量加载
	data := loadConfigFromEnv()
	if len(data) > 0 {
		c.data = data
		// 保存到文件
		if err := saveConfigToFile(c.configDir, data); err != nil {
			return fmt.Errorf("failed to save config from env: %w", err)
		}
		return nil
	}

	// 没有找到任何配置，使用空配置
	c.data = make(map[string]any)
	return nil
}

// getConfigDir 获取配置目录
func getConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(home, ".dscli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}
	return configDir, nil
}

// saveConfigToFile 保存配置到文件
func saveConfigToFile(configDir string, data map[string]any) error {
	if len(data) == 0 {
		return nil
	}

	var buf strings.Builder
	first := true
	for k, v := range data {
		if !first {
			buf.WriteString("\n\n")
		}
		first = false
		writeConfigValue(&buf, k, v, 0)
	}
	buf.WriteByte('\n')

	configFile := filepath.Join(configDir, "config.dscli")
	if err := os.WriteFile(configFile, []byte(buf.String()), 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// writeConfigValue 递归写入配置值为 nats-conf 格式。
// key = value 的简单值
// key { ... }  的嵌套 map
// key = [ ... ] 的数组
func writeConfigValue(w *strings.Builder, key string, val any, indent int) {
	prefix := strings.Repeat("  ", indent)

	switch v := val.(type) {
	case nil:
		return
	case map[string]any:
		if len(v) == 0 {
			fmt.Fprintf(w, "%s%s {}", prefix, key)
			return
		}
		fmt.Fprintf(w, "%s%s {\n", prefix, key)
		for k, subVal := range v {
			writeConfigValue(w, k, subVal, indent+1)
			w.WriteByte('\n')
		}
		fmt.Fprintf(w, "%s}", prefix)
	case []any:
		if len(v) == 0 {
			fmt.Fprintf(w, "%s%s = []", prefix, key)
			return
		}
		fmt.Fprintf(w, "%s%s = [\n", prefix, key)
		for _, item := range v {
			w.WriteString(prefix)
			w.WriteString("  ")
			writeArrayItem(w, item)
			w.WriteByte('\n')
		}
		fmt.Fprintf(w, "%s]", prefix)
	default:
		// 简单值：string、int64、float64、bool、time.Time 等
		fmt.Fprintf(w, "%s%s = %v", prefix, key, v)
	}
}

// writeArrayItem 写入数组项，支持内联 map 和简单值
func writeArrayItem(w *strings.Builder, val any) {
	switch v := val.(type) {
	case map[string]any:
		writeInlineMap(w, v)
	default:
		fmt.Fprint(w, val)
	}
}

// writeInlineMap 写入内联 map，格式 {key: value, key: value}
func writeInlineMap(w *strings.Builder, m map[string]any) {
	w.WriteByte('{')
	first := true
	for k, v := range m {
		if !first {
			w.WriteString(", ")
		}
		first = false
		fmt.Fprintf(w, "%s: %v", k, v)
	}
	w.WriteByte('}')
}

// // loadConfigFromFile 从文件加载配置
// func loadConfigFromFile(filename string) (map[string]any, error) {
// 	b, err := os.ReadFile(filename)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return Parse(string(b))
// }

// loadConfigFromEnv 从环境变量加载配置
func loadConfigFromEnv() map[string]any {
	const (
		BaseURL = "DEEPSEEK_BASE_URL"
		APIKey  = "DEEPSEEK_API_KEY"
	)

	config := make(map[string]any)

	baseURL := os.Getenv(BaseURL)
	if baseURL != "" {
		config[configName(BaseURL)] = baseURL
	}

	apiKey := os.Getenv(APIKey)
	if apiKey != "" {
		config[configName(APIKey)] = apiKey
	}

	return config
}

// configName 转换环境变量名为配置键名
func configName(envName string) string {
	if envName == "" {
		return ""
	}
	// DEEPSEEK_API_KEY -> deepseek-api-key
	name := strings.ReplaceAll(envName, "_", "-")
	name = strings.ToLower(name)
	name = strings.TrimPrefix(name, "export ")
	return name
}
