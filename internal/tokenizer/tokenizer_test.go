package tokenizer

import "testing"

func TestSanitizeFTS(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// English — words remain intact
		{"hello world", `"hello" "world"`},
		{"fix auth bug", `"fix" "auth" "bug"`},
		{"single", `"single"`},

		// Chinese — gse CutSearch produces compound + sub-words for recall
		{"全文搜索", `"全文" "搜索"`},
		{"中文分词", `"中文" "分词"`},
		{"Go单元测试", `"go" "单元" "测试" "单元测试"`},

		// Mixed
		{"中文 English 混合", `"中文" "english" "混合"`},
		{"编程 语言 Go", `"编程" "语言" "go"`},

		// Edge cases
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := SanitizeFTS(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFTS(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Chinese word segmentation (CutSearch = compound + sub-words)
		{"全文搜索", "全文 搜索"},
		{"中文分词效果需要验证", "中文 分词 效果 需要 验证"},

		// CutSearch produces sub-words for compound words;
		// stopwords (是/的/一个) are filtered.
		{"SQLite是一个轻量级的数据库", "sqlite 轻量 量级 轻量级 数据 据库 数据库"},
		{"Go单元测试", "go 单元 测试 单元测试"},
		{"JWT认证中间件实现", "jwt 认证 中间 中间件 实现"},
		{"中文 English 混合", "中文 english 混合"},

		// Pure English
		{"hello world", "hello world"},
		{"fix auth bug", "fix auth bug"},

		// Edge cases
		{"", ""},
		{"ABCD", "abcd"},
		{"测试123", "测试 123"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Tokenize(tt.input)
			if got != tt.want {
				t.Errorf("Tokenize(%q)\n  got:  %q\n  want: %q", tt.input, got, tt.want)
			}
		})
	}
}
