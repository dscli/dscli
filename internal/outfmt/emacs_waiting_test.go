package outfmt

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestGetEmacsAnimationInterval(t *testing.T) {
	tests := []struct {
		name           string
		emacsEnv       string
		insideEmacsEnv string
		want           time.Duration
	}{
		{"默认值", "", "", time.Second},
		{"EMACS=2", "2", "", 2 * time.Second},
		{"INSIDE_EMACS=3", "", "3", 3 * time.Second},
		{"EMACS优先", "1", "5", 1 * time.Second},
		{"无效值", "invalid", "", time.Second},
		{"零值", "0", "", time.Second},
		{"负值", "-1", "", time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			os.Unsetenv("EMACS")
			os.Unsetenv("INSIDE_EMACS")

			if tt.emacsEnv != "" {
				os.Setenv("EMACS", tt.emacsEnv)
			}
			if tt.insideEmacsEnv != "" {
				os.Setenv("INSIDE_EMACS", tt.insideEmacsEnv)
			}

			// 测试函数
			got := getEmacsAnimationInterval()

			// 验证结果
			if got != tt.want {
				t.Errorf("getEmacsAnimationInterval() = %v, want %v", got, tt.want)
			}

			// 清理环境变量
			os.Unsetenv("EMACS")
			os.Unsetenv("INSIDE_EMACS")
		})
	}
}

func TestIsEmacsEnvironment(t *testing.T) {
	tests := []struct {
		name           string
		emacsEnv       string
		insideEmacsEnv string
		want           bool
	}{
		{"无环境变量", "", "", false},
		{"只有EMACS", "1", "", true},
		{"只有INSIDE_EMACS", "", "1", true},
		{"两者都有", "1", "1", true},
		{"空值", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			os.Unsetenv("EMACS")
			os.Unsetenv("INSIDE_EMACS")

			if tt.emacsEnv != "" {
				os.Setenv("EMACS", tt.emacsEnv)
			}
			if tt.insideEmacsEnv != "" {
				os.Setenv("INSIDE_EMACS", tt.insideEmacsEnv)
			}

			// 测试函数
			got := isEmacsEnvironment()

			// 验证结果
			if got != tt.want {
				t.Errorf("isEmacsEnvironment() = %v, want %v", got, tt.want)
			}

			// 清理环境变量
			os.Unsetenv("EMACS")
			os.Unsetenv("INSIDE_EMACS")
		})
	}
}

func TestShowEmacsAnimationIntegration(t *testing.T) {
	// 这是一个集成测试，验证动画函数的基本功能
	t.Run("正常完成", func(t *testing.T) {
		ctx := t.Context()

		done := make(chan bool)

		// 设置快速测试间隔
		os.Setenv("EMACS", "1") // 1秒间隔，但我们会很快完成

		// 启动动画（在goroutine中）
		go func() {
			showEmacsAnimation(ctx, done)
		}()

		// 立即发送完成信号
		time.Sleep(50 * time.Millisecond)
		done <- true

		// 给一点时间让goroutine结束
		time.Sleep(100 * time.Millisecond)

		os.Unsetenv("EMACS")
	})

	t.Run("取消上下文", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		done := make(chan bool)

		// 设置快速测试间隔
		os.Setenv("EMACS", "1")

		// 启动动画
		go func() {
			showEmacsAnimation(ctx, done)
		}()

		// 立即取消
		time.Sleep(50 * time.Millisecond)
		cancel()

		// 给一点时间让goroutine结束
		time.Sleep(100 * time.Millisecond)

		os.Unsetenv("EMACS")
	})
}

func TestIsEmacsEnvironmentWithConfig(t *testing.T) {
	tests := []struct {
		name           string
		emacsEnv       string
		insideEmacsEnv string
		wantIsEmacs    bool
		wantInterval   time.Duration
	}{
		{"无环境变量", "", "", false, time.Second},
		{"EMACS=2", "2", "", true, 2 * time.Second},
		{"INSIDE_EMACS=3", "", "3", true, 3 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置环境变量
			os.Unsetenv("EMACS")
			os.Unsetenv("INSIDE_EMACS")

			if tt.emacsEnv != "" {
				os.Setenv("EMACS", tt.emacsEnv)
			}
			if tt.insideEmacsEnv != "" {
				os.Setenv("INSIDE_EMACS", tt.insideEmacsEnv)
			}

			// 测试函数
			gotIsEmacs, gotInterval := isEmacsEnvironmentWithConfig()

			// 验证结果
			if gotIsEmacs != tt.wantIsEmacs {
				t.Errorf("isEmacsEnvironmentWithConfig() isEmacs = %v, want %v", gotIsEmacs, tt.wantIsEmacs)
			}
			if gotInterval != tt.wantInterval {
				t.Errorf("isEmacsEnvironmentWithConfig() interval = %v, want %v", gotInterval, tt.wantInterval)
			}

			// 清理环境变量
			os.Unsetenv("EMACS")
			os.Unsetenv("INSIDE_EMACS")
		})
	}
}
