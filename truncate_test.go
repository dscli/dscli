package main

import (
	"strings"
	"testing"
)

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string // description of this test case
		s      string
		maxLen int
		want   string
	}{
		// 空字符串和边界情况
		{"Empty", "", 5, ""},
		{"MaxLen negative", "hello", -1, ""},
		{"MaxLen zero", "hello", 0, ""},
		{"MaxLen 1", "he", 1, ""},
		{"MaxLen 2", "hello", 2, ""},

		// 不截断情况
		{"MaxLen greater than length", "hello world", 12, "hello world"},
		{"Pure English", "hello world", 11, "hello world"},
		{"Short string", "short", 10, "short"},

		// 需要截断的情况
		{"English Truncate", "hello world", 10, "hello w..."},
		{"Chinese", "世界，你好", 5, "世界，你好"},
		{"Chinese Truncate", "世界，你好！", 4, "世..."},

		// 边界精确值
		{"MaxLen exactly 3, string longer", "hello", 3, "..."},
		{"MaxLen exactly 3, string shorter", "Hi", 3, "Hi"},
		{"MaxLen 4 with Chinese", "你好世界", 4, "你好世界"},

		// Unicode特殊情况
		{"Emoji", "Hello 😊 World", 8, "Hello..."},
		{"Emoji truncate", "Hello 😊 World", 7, "Hell..."},
		{"MaxLen 2 with Chinese", "你好", 2, ""},

		// 长字符串性能
		{"Long string", strings.Repeat("a", 100), 50, strings.Repeat("a", 47) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateString(%q, %d) = %q, want %q",
					tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
