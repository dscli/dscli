package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// sanitizeDeepSeekEnv clears all DEEPSEEK_* environment variables that
// loadConfigFromEnv() may read, so tests never pick up ambient secrets:
// a real API key in the developer's shell would otherwise flow into
// cfg.data via the env fallback and leak into test output on failure.
func sanitizeDeepSeekEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"DEEPSEEK_API_KEY",
		"DEEPSEEK_BASE_URL",
	} {
		t.Setenv(key, "")
	}
}

func TestConfig_Get(t *testing.T) {
	sanitizeDeepSeekEnv(t)

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
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			tempDir := t.TempDir()

			cfg, err := NewWithDir(tempDir)
			if err != nil {
				t.Fatalf("NewWithDir() error = %v", err)
			}

			got := cfg.Get(tt.configKey, tt.defaultValue)
			if got != tt.want {
				t.Errorf("Config.Get() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_SaveAndLoad(t *testing.T) {
	sanitizeDeepSeekEnv(t)

	tempDir := t.TempDir()

	cfg, err := NewWithDir(tempDir)
	if err != nil {
		t.Fatalf("NewWithDir() error = %v", err)
	}

	cfg.Set("deepseek-api-key", "sk-save-test")
	cfg.Set("deepseek-base-url", "https://api.save.test")

	if err := cfg.Save(); err != nil {
		t.Fatalf("Config.Save() error = %v", err)
	}

	configFile := filepath.Join(tempDir, "config.dscli")
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Fatalf("config file not created: %v", err)
	}

	cfg2, err := NewWithDir(tempDir)
	if err != nil {
		t.Fatalf("NewWithDir() second time error = %v", err)
	}

	if got := cfg2.Get("deepseek-api-key", ""); got != "sk-save-test" {
		t.Errorf("reloaded api key = %v, want sk-save-test", got)
	}
	if got := cfg2.Get("deepseek-base-url", ""); got != "https://api.save.test" {
		t.Errorf("reloaded base url = %v, want https://api.save.test", got)
	}
}

func TestGlobalGet(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-global-test")

	got := Get("deepseek-api-key", "default")

	if got == "default" {
		t.Log("Global config already initialized, using cached values")
	} else if !strings.HasPrefix(got, "sk-") {
		t.Errorf("global Get() = %v, expected API key or default", got)
	}
}

func TestConfig_SanitizePreventsAmbientLeak(t *testing.T) {
	// Simulate a developer machine with real credentials in the shell.
	t.Setenv("DEEPSEEK_API_KEY", "sk-ambient-secret")
	t.Setenv("DEEPSEEK_BASE_URL", "https://ambient.invalid")

	// Tests constructing a Config must sanitize the environment first;
	// otherwise load() picks up the ambient values via loadConfigFromEnv.
	sanitizeDeepSeekEnv(t)

	cfg, err := NewWithDir(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithDir() error = %v", err)
	}
	if got := cfg.Get("deepseek-api-key", "default"); got != "default" {
		t.Errorf("Config.Get() picked up ambient API key: %v", got)
	}
}

func TestSaveConfigToFile_NestedMap(t *testing.T) {
	tempDir := t.TempDir()

	data := map[string]any{
		"authorization": map[string]any{
			"user":     "admin",
			"password": "secret",
			"timeout":  float64(1.5),
		},
	}

	if err := saveConfigToFile(tempDir, data); err != nil {
		t.Fatalf("saveConfigToFile() error = %v", err)
	}

	loaded, err := ParseFile(filepath.Join(tempDir, "config.dscli"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	auth, ok := loaded["authorization"].(map[string]any)
	if !ok {
		t.Fatal("authorization is not a map")
	}
	if got, want := auth["user"], "admin"; got != want {
		t.Errorf("user = %v, want %v", got, want)
	}
	if got, want := auth["password"], "secret"; got != want {
		t.Errorf("password = %v, want %v", got, want)
	}
	if got, want := auth["timeout"], float64(1.5); got != want {
		t.Errorf("timeout = %v, want %v", got, want)
	}
}

func TestSaveConfigToFile_Array(t *testing.T) {
	tempDir := t.TempDir()

	data := map[string]any{
		"servers": []any{"a.com", "b.com", "c.com"},
	}

	if err := saveConfigToFile(tempDir, data); err != nil {
		t.Fatalf("saveConfigToFile() error = %v", err)
	}

	loaded, err := ParseFile(filepath.Join(tempDir, "config.dscli"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	servers, ok := loaded["servers"].([]any)
	if !ok {
		t.Fatal("servers is not an array")
	}
	if len(servers) != 3 {
		t.Fatalf("servers length = %d, want 3", len(servers))
	}
	if got, want := servers[0], "a.com"; got != want {
		t.Errorf("servers[0] = %v, want %v", got, want)
	}
	if got, want := servers[1], "b.com"; got != want {
		t.Errorf("servers[1] = %v, want %v", got, want)
	}
	if got, want := servers[2], "c.com"; got != want {
		t.Errorf("servers[2] = %v, want %v", got, want)
	}
}

func TestSaveConfigToFile_ArrayOfMaps(t *testing.T) {
	tempDir := t.TempDir()

	data := map[string]any{
		"users": []any{
			map[string]any{"user": "alice", "role": "admin"},
			map[string]any{"user": "bob", "role": "user"},
		},
	}

	if err := saveConfigToFile(tempDir, data); err != nil {
		t.Fatalf("saveConfigToFile() error = %v", err)
	}

	loaded, err := ParseFile(filepath.Join(tempDir, "config.dscli"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	users, ok := loaded["users"].([]any)
	if !ok {
		t.Fatal("users is not an array")
	}
	if len(users) != 2 {
		t.Fatalf("users length = %d, want 2", len(users))
	}

	u0, ok := users[0].(map[string]any)
	if !ok {
		t.Fatal("users[0] is not a map")
	}
	if got, want := u0["user"], "alice"; got != want {
		t.Errorf("users[0].user = %v, want %v", got, want)
	}
	if got, want := u0["role"], "admin"; got != want {
		t.Errorf("users[0].role = %v, want %v", got, want)
	}

	u1, ok := users[1].(map[string]any)
	if !ok {
		t.Fatal("users[1] is not a map")
	}
	if got, want := u1["user"], "bob"; got != want {
		t.Errorf("users[1].user = %v, want %v", got, want)
	}
	if got, want := u1["role"], "user"; got != want {
		t.Errorf("users[1].role = %v, want %v", got, want)
	}
}

func TestSaveConfigToFile_EmptyMapAndArray(t *testing.T) {
	tempDir := t.TempDir()

	data := map[string]any{
		"emptymap":   map[string]any{},
		"emptyarray": []any{},
		"valid":      "ok",
	}

	if err := saveConfigToFile(tempDir, data); err != nil {
		t.Fatalf("saveConfigToFile() error = %v", err)
	}

	loaded, err := ParseFile(filepath.Join(tempDir, "config.dscli"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if _, ok := loaded["emptymap"]; !ok {
		t.Error("emptymap key missing")
	}
	if _, ok := loaded["emptyarray"]; !ok {
		t.Error("emptyarray key missing")
	}
	if got, want := loaded["valid"], "ok"; got != want {
		t.Errorf("valid = %v, want %v", got, want)
	}
}

func TestGetStrings(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{name: "未配置", value: nil, want: nil},
		{name: "类型不匹配", value: "emacs", want: nil},
		{name: "空数组", value: []any{}, want: nil},
		{name: "正常数组", value: []any{"emacs", "--eval", "(dscli--send-message-raw)"}, want: []string{"emacs", "--eval", "(dscli--send-message-raw)"}},
		{name: "跳过非字符串和空字符串", value: []any{"emacs", 42, "", "chat"}, want: []string{"emacs", "chat"}},
		{name: "过滤后为空", value: []any{"", 42}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetValue("teststrings", tt.value)
			got := GetStrings("teststrings")
			if !slices.Equal(got, tt.want) {
				t.Errorf("GetStrings() = %v, want %v", got, tt.want)
			}
		})
	}
}
