package main

import (
	"context"
	"testing"
	"time"
)

func TestCodeMakeFormat(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		want    string
		wantErr bool
	}{
		{"normal test", "done\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.WithValue(t.Context(), CodeFormatKey, "echo done")
			got, gotErr := CodeMakeFormat(ctx)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CodeMakeFormat() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CodeMakeFormat() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("CodeMakeFormat() = %v", got)
			}
		})
	}
}

func TestCodeMakeFormatWithTimeout(t *testing.T) {
	t.Run("normal timeout", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), CodeFormatKey, "echo done")
		_, err := CodeMakeFormatWithTimeout(ctx, 1*time.Second)
		if err != nil {
			t.Errorf("CodeMakeFormatWithTimeout() failed: %v", err)
		}
	})

	t.Run("very short timeout", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), CodeFormatKey, "sleep 0.01\necho done")
		// 使用极短的超时，期望超时错误
		_, err := CodeMakeFormatWithTimeout(ctx, 50*time.Microsecond)
		if err == nil {
			t.Error("CodeMakeFormatWithTimeout() should have timed out")
		}
	})
}

func TestCodeMakeFormatSafe(t *testing.T) {
	t.Run("safe version", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), CodeFormatKey, "echo done")
		_, err := CodeMakeFormatSafe(ctx)
		if err != nil {
			t.Errorf("CodeMakeFormatSafe() failed: %v", err)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx := context.WithValue(t.Context(), CodeFormatKey, "sleep 0.01\necho done")
		ctx, cancel := context.WithCancel(ctx)
		cancel() // 立即取消

		_, err := CodeMakeFormatSafe(ctx)
		if err == nil {
			t.Error("CodeMakeFormatSafe() should have failed with cancelled context")
		}
	})
}
