package config

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

var (
	globalConfig     *Config
	globalConfigOnce sync.Once
)

// Get 获取配置值（向后兼容接口）
// 使用全局配置实例，支持懒加载
func Get(name, defaultValue string, alias ...string) string {
	return getGlobalConfig().Get(name, defaultValue, alias...)
}

// GetBool 获取布尔配置值
// 支持 strconv.ParseBool 的格式：1/t/T/TRUE/true/True → true，0/f/F/FALSE/false/False → false
// 未配置或解析失败时返回 defaultValue
func GetBool(name string, defaultValue bool, alias ...string) bool {
	s := getGlobalConfig().Get(name, "", alias...)
	if s == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(s)
	if err != nil {
		return defaultValue
	}
	return b
}

// GetInt 获取整数配置值
// 未配置或解析失败时返回 defaultValue
func GetInt(name string, defaultValue int, alias ...string) int {
	s := getGlobalConfig().Get(name, "", alias...)
	if s == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return defaultValue
	}
	return n
}

// ConfigDir 配置目录（向后兼容变量）
// 注意：这是一个函数调用，返回配置目录路径
var ConfigDir = func() string {
	return getGlobalConfig().ConfigDir()
}()

// getGlobalConfig 获取全局配置实例
func getGlobalConfig() *Config {
	globalConfigOnce.Do(func() {
		var err error
		globalConfig, err = New()
		if err != nil {
			// 记录错误但不panic，使用空配置继续运行
			globalConfig = &Config{
				data:      make(map[string]string),
				configDir: defaultConfigDir(),
			}
		}
	})
	return globalConfig
}

// defaultConfigDir 获取默认配置目录（当无法创建时使用）
func defaultConfigDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = "."
	}
	return filepath.Join(home, ".dscli")
}
