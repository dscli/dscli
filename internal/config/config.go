package config

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	globalConfig     *Config
	globalConfigOnce sync.Once
)
// Get 获取配置值（向后兼容接口）
// 使用全局配置实例，支持懒加载
func Get(name string, defaultValue string) string {
	return getGlobalConfig().Get(name, defaultValue)
}

// ConfigDir 配置目录（向后兼容变量）
// 注意：这是一个函数调用，返回配置目录路径
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