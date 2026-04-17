package config

import (
	"strings"
	"testing"
)

func TestGetConfigDir(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"Get", ".dscli"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getConfigDir()
			if !strings.HasSuffix(got, tt.want) {
				t.Fatal(got, tt.want)
			}
			if got != ConfigDir {
				t.Fatal(got, ConfigDir)
			}
		})
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		desc string
		name string
		want    string
	}{
		{"api-key", "deepseek-api-key", "sk-xxxx"},
		{"base-url", "deepseek-base-url", "https://api.deepseek.com"},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Cleanup(func() func() {
				ConfigDir = t.TempDir()
				t.Setenv("DEEPSEEK_API_KEY", "sk-xxxx")
				t.Setenv("DEEPSEEK_BASE_URL", "https://api.deepseek.com")
				_config = loadConfig()
				return func() {
					ConfigDir = getConfigDir()
					_config = loadConfig()
				}
			}())
			got := Get(tt.name, "")
			if !strings.HasPrefix(got, tt.want) {
				t.Fatal(got)
			}
		})
	}
}
