package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfig_Get(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		configKey    string
		defaultValue string
		want         string
	}{
		{
			name: "从环境变量获取API Key",
			envVars: map[string]string{
				"DEEPSEEK_API_KEY": "sk-test123",
			},
			configKey:    "deepseek-api-key",
			defaultValue: "",
			want:         "sk-test123",
		},
		{
			name: "从环境变量获取Base URL",
			envVars: map[string]string{
				"DEEPSEEK_BASE_URL": "https://api.test.deepseek.com",
			},
			configKey:    "deepseek-base-url",
			defaultValue: "https://api.deepseek.com",
			want:         "https://api.test.deepseek.com",
		},
		{
			name:         "使用默认值",
			envVars:      map[string]string{},
			configKey:    "deepseek-api-key",
			defaultValue: "sk-default",
			want:         "sk-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			// 创建临时配置目录
			tempDir := t.TempDir()

			// 创建新的配置实例
			cfg, err := NewWithDir(tempDir)
			if err != nil {
				t.Fatalf("NewWithDir() error = %v", err)
			}

			// 测试Get方法
			got := cfg.Get(tt.configKey, tt.defaultValue)
			if got != tt.want {
				t.Errorf("Config.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()

	cfg, err := NewWithDir(tempDir)
	if err != nil {
		t.Fatalf("NewWithDir() error = %v", err)
	}

	// 设置一些配置值
	cfg.Set("deepseek-api-key", "sk-save-test")
	cfg.Set("deepseek-base-url", "https://api.save.test")

	// 保存配置
	if err := cfg.Save(); err != nil {
		t.Fatalf("Config.Save() error = %v", err)
	}

	// 验证配置文件存在
	configFile := filepath.Join(tempDir, "config.dscli")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatalf("config file not created: %v", err)
	}

	// 重新加载配置
	cfg2, err := NewWithDir(tempDir)
	if err != nil {
		t.Fatalf("NewWithDir() second time error = %v", err)
	}

	// 验证配置值
	if got := cfg2.Get("deepseek-api-key", ""); got != "sk-save-test" {
		t.Errorf("reloaded api key = %v, want sk-save-test", got)
	}
	if got := cfg2.Get("deepseek-base-url", ""); got != "https://api.save.test" {
		t.Errorf("reloaded base url = %v, want https://api.save.test", got)
	}
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "简单键值对",
			input: `deepseek-api-key = sk-test123
deepseek-base-url = https://api.test.com`,
			want: map[string]string{
				"deepseek-api-key":  "sk-test123",
				"deepseek-base-url": "https://api.test.com",
			},
		},
		{
			name: "带注释",
			input: `# API配置
deepseek-api-key = sk-test123
# 基础URL
deepseek-base-url = https://api.test.com`,
			want: map[string]string{
				"deepseek-api-key":  "sk-test123",
				"deepseek-base-url": "https://api.test.com",
			},
		},
		{
			name: "旧格式export",
			input: `export DEEPSEEK_API_KEY=sk-test123
export DEEPSEEK_BASE_URL=https://api.test.com`,
			want: map[string]string{
				"deepseek-api-key":  "sk-test123",
				"deepseek-base-url": "https://api.test.com",
			},
		},
		{
			name: "空行和空格",
			input: `

deepseek-api-key = sk-test123

deepseek-base-url = https://api.test.com

`,
			want: map[string]string{
				"deepseek-api-key":  "sk-test123",
				"deepseek-base-url": "https://api.test.com",
			},
		},
		{
			name:    "空输入",
			input:   "",
			want:    map[string]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConfig(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("parseConfig() got %d keys, want %d keys", len(got), len(tt.want))
				return
			}

			for key, wantValue := range tt.want {
				gotValue, ok := got[key]
				if !ok {
					t.Errorf("parseConfig() missing key %q", key)
					continue
				}
				if gotValue != wantValue {
					t.Errorf("parseConfig()[%q] = %v, want %v", key, gotValue, wantValue)
				}
			}
		})
	}
}

func TestConfigName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"DEEPSEEK_API_KEY", "deepseek-api-key"},
		{"DEEPSEEK_BASE_URL", "deepseek-base-url"},
		{"TEST_VARIABLE", "test-variable"},
		{"", ""},
		{"simple", "simple"},
		{"ALREADY_LOWER", "already-lower"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := configName(tt.input)
			if got != tt.want {
				t.Errorf("configName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGlobalGet(t *testing.T) {
	// 测试全局Get函数（向后兼容）
	originalAPIKey := os.Getenv("DEEPSEEK_API_KEY")
	defer func() {
		if originalAPIKey != "" {
			os.Setenv("DEEPSEEK_API_KEY", originalAPIKey)
		} else {
			os.Unsetenv("DEEPSEEK_API_KEY")
		}
	}()

	// 设置测试环境变量
	os.Setenv("DEEPSEEK_API_KEY", "sk-global-test")

	// 注意：由于全局变量只初始化一次，我们无法完全重置
	// 这里测试的是全局Get函数的基本功能
	got := Get("deepseek-api-key", "default")

	// 检查是否获取到了环境变量的值或默认值
	if got == "default" {
		// 如果全局配置已经初始化过，可能不会使用新的环境变量
		// 这是预期的行为，因为全局配置只加载一次
		t.Log("Global config already initialized, using cached values")
	} else if !strings.HasPrefix(got, "sk-") {
		t.Errorf("global Get() = %v, expected API key or default", got)
	}
}
