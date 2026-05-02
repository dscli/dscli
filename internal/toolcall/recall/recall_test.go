package recall

import (
	"strings"
	"testing"
	"time"
)

func TestFormatTime(t *testing.T) {
	now := time.Now()

	today := now.Format("15:04")
	if got := FormatTime(now); got != today {
		t.Errorf("FormatTime(today) = %s, want %s", got, today)
	}

	thisYear := time.Date(now.Year(), 6, 15, 10, 30, 0, 0, now.Location())
	wantThisYear := thisYear.Format("01-02 15:04")
	if got := FormatTime(thisYear); got != wantThisYear {
		t.Errorf("FormatTime(this year) = %s, want %s", got, wantThisYear)
	}

	otherYear := time.Date(2024, 12, 1, 8, 0, 0, 0, now.Location())
	wantOtherYear := otherYear.Format("2006-01-02 15:04")
	if got := FormatTime(otherYear); got != wantOtherYear {
		t.Errorf("FormatTime(other year) = %s, want %s", got, wantOtherYear)
	}
}

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
	// 基本烟雾测试：搜索一个不可能存在的词
	results, err := SearchMessages([]string{"xyznonexistent12345"}, 1, 5, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("预期0结果，实际 %d", len(results))
	}

	// 测试空关键词报错
	_, err = SearchMessages([]string{""}, 1, 5, false, 0)
	if err == nil || !strings.Contains(err.Error(), "没有有效的搜索关键词") {
		t.Errorf("空关键词应该报错: %v", err)
	}

	// 测试空输入报错
	_, err = SearchMessages(nil, 1, 5, false, 0)
	if err == nil || !strings.Contains(err.Error(), "至少需要一个搜索关键词") {
		t.Errorf("nil 关键词应该报错: %v", err)
	}
}
