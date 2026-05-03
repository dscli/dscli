package history

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTruncate(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		want    string
	}{
		{"短文本不截断", "hello world", 20, "hello world"},
		{"刚好等于", "12", 2, "12"},
		{"需要截断", "hello world", 5, "hello..."},
		{"中文截断", "你好世界测试中文", 4, "你好世界..."},
		{"合并空白", "a   b\n\nc", 20, "a b c"},
		{"前导空白", "   hello", 10, "hello"},
		{"空字符串", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Truncate(tt.content, tt.maxLen)
			if got != tt.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tt.content, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestSearchMessages(t *testing.T) {
	// 烟雾测试：搜索一个不可能存在的词（加时间戳确保唯一性）
	bogusKeyword := fmt.Sprintf("smoketest-no-match-%s-x", time.Now().Format("20060102-150405"))
	results, err := SearchMessages(context.Background(), []string{bogusKeyword}, 1, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		for _, r := range results {
			t.Logf("unexpected match: ID=%d Role=%s Content=%q", r.Message.ID, r.Message.Role, Truncate(r.Message.Content, 80))
		}
		t.Errorf("预期0结果，实际 %d", len(results))
	}

	// 测试空关键词报错
	_, err = SearchMessages(context.Background(), []string{""}, 1, 5)
	if err == nil || !strings.Contains(err.Error(), "没有有效的搜索关键词") {
		t.Errorf("空关键词应该报错: %v", err)
	}

	// 测试空输入报错
	_, err = SearchMessages(context.Background(), nil, 1, 5)
	if err == nil || !strings.Contains(err.Error(), "至少需要一个搜索关键词") {
		t.Errorf("nil 关键词应该报错: %v", err)
	}
}
