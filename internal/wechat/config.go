package wechat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config 微信客户端配置
type Config struct {
	// 账号配置
	Account string `json:"account"` // 微信账号标识
	Mode    string `json:"mode"`    // "desktop" 或 "web"

	// 存储配置
	DBPath string `json:"db_path"` // 数据库路径

	// 登录配置
	AutoLogin bool `json:"auto_login"` // 是否自动尝试热登录
	PushLogin bool `json:"push_login"` // 是否优先使用免扫码登录

	// 消息配置
	ReplyDelay   int `json:"reply_delay"`    // 回复延迟（毫秒）
	MaxMsgLength int `json:"max_msg_length"` // 最大消息长度

	// 安全配置
	AllowedFriends []string `json:"allowed_friends"` // 白名单好友
	BlockedUsers   []string `json:"blocked_users"`   // 黑名单用户
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	homeDir, _ := os.UserHomeDir()
	return &Config{
		Account:        "default",
		Mode:           "desktop",
		DBPath:         filepath.Join(homeDir, ".dscli", "wechat.db"),
		AutoLogin:      true,
		PushLogin:      true,
		ReplyDelay:     1000,
		MaxMsgLength:   5000,
		AllowedFriends: []string{},
		BlockedUsers:   []string{},
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Account == "" {
		return fmt.Errorf("账号不能为空")
	}

	if c.Mode != "desktop" && c.Mode != "web" {
		return fmt.Errorf("模式必须是 'desktop' 或 'web'")
	}

	if c.DBPath == "" {
		return fmt.Errorf("数据库路径不能为空")
	}

	if c.ReplyDelay < 0 {
		return fmt.Errorf("回复延迟不能为负数")
	}

	if c.MaxMsgLength <= 0 {
		return fmt.Errorf("最大消息长度必须大于0")
	}

	return nil
}

// LoadConfig 从文件加载配置
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, wrapErr(err, "读取配置文件失败")
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, wrapErr(err, "解析配置文件失败")
	}

	// 合并默认配置
	defaultConfig := DefaultConfig()
	if config.Account == "" {
		config.Account = defaultConfig.Account
	}
	if config.Mode == "" {
		config.Mode = defaultConfig.Mode
	}
	if config.DBPath == "" {
		config.DBPath = defaultConfig.DBPath
	}

	if err := config.Validate(); err != nil {
		return nil, wrapErr(err, "配置验证失败")
	}

	return &config, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(config *Config, path string) error {
	if err := config.Validate(); err != nil {
		return wrapErr(err, "配置验证失败")
	}

	// 确保目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return wrapErr(err, "创建配置目录失败")
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return wrapErr(err, "序列化配置失败")
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return wrapErr(err, "写入配置文件失败")
	}

	return nil
}

// GetDefaultConfigPath 获取默认配置文件路径
func GetDefaultConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".dscli", "wechat.json")
}
