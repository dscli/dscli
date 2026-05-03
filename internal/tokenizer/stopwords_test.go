package tokenizer

import "testing"

// TestIsStop verifies that common Chinese function words are recognized
// as stopwords and that content words are not.
func TestIsStop(t *testing.T) {
	// Must be stopwords (high-frequency function words).
	stops := []string{"的", "了", "也", "吧", "吗", "呢", "是", "一个", "啊", "着", "在", "和", "就", "都", "而", "及"}
	for _, w := range stops {
		if !IsStop(w) {
			t.Errorf("IsStop(%q) = false, want true", w)
		}
	}

	// Must NOT be stopwords (content words).
	content := []string{"Go", "hello", "SQLite", "测试", "数据库", "搜索", "需要", "实现", "中间", "单元测试", "全文搜索"}
	for _, w := range content {
		if IsStop(w) {
			t.Errorf("IsStop(%q) = true, want false", w)
		}
	}

	// Edge cases.
	if IsStop("") {
		t.Error("IsStop(\"\") = true, want false")
	}
}
