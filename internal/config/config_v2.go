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
	data      map[string]string
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
		data:      make(map[string]string),
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
		data:      make(map[string]string),
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
		return value
	}

	for _, name = range alias {
		if value, ok := c.data[name]; ok && value != "" {
			return value
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
	if data, err := loadConfigFromFile(configFile); err == nil && len(data) > 0 {
		c.data = data
		return nil
	}

	// 尝试从旧格式文件加载
	oldConfigFile := filepath.Join(c.configDir, "dscli.env")
	if data, err := loadConfigFromFile(oldConfigFile); err == nil && len(data) > 0 {
		c.data = data
		// 自动迁移到新格式
		if err := saveConfigToFile(c.configDir, data); err != nil {
			return fmt.Errorf("failed to migrate config: %w", err)
		}
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
	c.data = make(map[string]string)
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
func saveConfigToFile(configDir string, data map[string]string) error {
	if len(data) == 0 {
		return nil
	}

	lines := []string{}
	for k, v := range data {
		line := fmt.Sprintf("%s = %s", k, v)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n\n")
	configFile := filepath.Join(configDir, "config.dscli")

	if err := os.WriteFile(configFile, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}

// loadConfigFromFile 从文件加载配置
func loadConfigFromFile(filename string) (map[string]string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return parseConfig(string(b))
}

// loadConfigFromEnv 从环境变量加载配置
func loadConfigFromEnv() map[string]string {
	const (
		BaseURL = "DEEPSEEK_BASE_URL"
		APIKey  = "DEEPSEEK_API_KEY"
	)

	config := make(map[string]string)

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
	return strings.ToLower(name)
}

// parseConfig 解析配置内容
// parseConfig 解析配置内容
func parseConfig(data string) (map[string]string, error) {
	config := make(map[string]string)

	// 使用兼容的字符串分割方式
	lines := strings.Split(data, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 处理注释
		if commentIdx := strings.Index(line, "#"); commentIdx != -1 {
			line = line[:commentIdx]
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
		}

		// 查找等号分隔符
		before, after, ok := strings.Cut(line, "=")
		if !ok {
			// 尝试支持 "export KEY = VALUE" 格式
			fields := strings.Fields(line)
			if len(fields) >= 4 && fields[0] == "export" && fields[2] == "=" {
				key := fields[1]
				value := strings.Join(fields[3:], " ")
				config[configName(key)] = value
			} else if len(fields) >= 3 && fields[1] == "=" {
				// 支持 "KEY = VALUE" 格式
				key := fields[0]
				value := strings.Join(fields[2:], " ")
				config[configName(key)] = value
			}
			continue
		}

		// 解析 "KEY=VALUE" 或 "KEY = VALUE" 格式
		key := strings.TrimSpace(before)
		value := strings.TrimSpace(after)

		// 处理 "export KEY=VALUE" 格式
		if strings.HasPrefix(key, "export ") {
			key = strings.TrimSpace(strings.TrimPrefix(key, "export"))
		}

		if key != "" {
			config[configName(key)] = value
		}
	}

	return config, nil
}
