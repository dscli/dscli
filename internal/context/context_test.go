package context

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBoolValue(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		v    any
		d    bool
		want bool
	}{
		{"Normal usage", true, false, true},
		{"nil", nil, false, false},
		{"false", false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := ContextKeyType[bool]{"boolKey"}
			ctx := context.Background()
			if tt.v != nil {
				ctx = context.WithValue(ctx, k, tt.v)
			}
			got := ContextValue(ctx, k, tt.d)
			if got != tt.want {
				t.Errorf("BoolValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringValue(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		v    any
		d    string
		want string
	}{
		{"normal usage", "yes", "", "yes"},
		{"nil", nil, "", ""},
		{"empty", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := ContextKeyType[string]{"stringKey"}
			ctx := context.Background()
			if tt.v != nil {
				ctx = context.WithValue(ctx, k, tt.v)
			}
			got := ContextValue(ctx, k, tt.d)
			if got != tt.want {
				t.Errorf("StringValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTimeValue(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		v any
		d time.Time
	}{
		{"normal usage", time.Now(), time.Time{}},
		{"nil", nil, time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			k := ContextKeyType[time.Time]{"timeKey"}
			if tt.v != nil {
				ctx = context.WithValue(ctx, k, tt.v)
			}
			got := ContextValue(ctx, k, tt.d)
			if tt.v != nil && got.IsZero() {
				t.Errorf("TimeValue() = %v", got)
			}
			if tt.v == nil && !got.IsZero() {
				t.Errorf("TimeValue() = %v", got)
			}
		})
	}
}

// TestGetProjectRootPreservesCWD 验证 GetProjectRoot 不会改变进程的 CWD。
//
// v0.7.7 回归修复：之前 GetProjectRoot 在发现 CWD 不等于
// projectRoot 时会强制 chdir，导致用户传入的相对路径（如 skill add
// ascend-docker）被错误地相对于项目根目录解析。
func TestGetProjectRootPreservesCWD(t *testing.T) {
	cwdBefore, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取 CWD 失败: %v", err)
	}

	// 调用 GetProjectRoot
	_ = GetProjectRoot()

	cwdAfter, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取 CWD 失败: %v", err)
	}

	if cwdBefore != cwdAfter {
		t.Errorf("GetProjectRoot 不应改变 CWD:\n  before: %s\n  after:  %s", cwdBefore, cwdAfter)
	}
}

// TestGetProjectRootReturnsAbsPath 验证返回的是绝对路径。
func TestGetProjectRootReturnsAbsPath(t *testing.T) {
	root := GetProjectRoot()
	if root == "" {
		t.Fatal("GetProjectRoot 返回空字符串")
	}
	if !filepath.IsAbs(root) {
		t.Errorf("GetProjectRoot 应返回绝对路径，得到: %s", root)
	}
}
