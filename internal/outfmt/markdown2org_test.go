package outfmt

import (
	"strings"
	"testing"
)

func TestMarkdownToOrgConverter_ConvertLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// 标题测试
		{
			name:     "一级标题",
			input:    "# Heading 1\n",
			expected: "* Heading 1\n",
		},
		{
			name:     "二级标题",
			input:    "## Heading 2\n",
			expected: "** Heading 2\n",
		},
		{
			name:     "三级标题",
			input:    "### Heading 3\n",
			expected: "*** Heading 3\n",
		},
		// 粗体测试
		{
			name:     "粗体文本",
			input:    "This is **bold** text\n",
			expected: "This is *bold* text\n",
		},
		{
			name:     "多个粗体",
			input:    "**bold1** and **bold2**\n",
			expected: "*bold1* and *bold2*\n",
		},
		// 斜体测试
		{
			name:     "斜体文本",
			input:    "This is *italic* text\n",
			expected: "This is /italic/ text\n",
		},
		// 删除线测试
		{
			name:     "删除线文本",
			input:    "This is ~~strikethrough~~ text\n",
			expected: "This is +strikethrough+ text\n",
		},
		// 内联代码测试
		{
			name:     "内联代码",
			input:    "Use `fmt.Println` function\n",
			expected: "Use =fmt.Println= function\n",
		},
		// 链接测试
		{
			name:     "链接",
			input:    "Visit [Google](https://google.com)\n",
			expected: "Visit [[https://google.com][Google]]\n",
		},
		// 混合测试
		{
			name:     "混合格式",
			input:    "# Title with **bold** and *italic*\n",
			expected: "* Title with *bold* and /italic/\n",
		},
		// 列表测试（保持不变）
		{
			name:     "无序列表",
			input:    "- Item 1\n",
			expected: "- Item 1\n",
		},
		{
			name:     "有序列表",
			input:    "1. First item\n",
			expected: "1. First item\n",
		},
		// 代码块测试
		{
			name:     "代码块开始",
			input:    "```go\n",
			expected: "#+begin_src go\n",
		},
		{
			name:     "代码块开始（无语言）",
			input:    "```\n",
			expected: "#+begin_src text\n",
		},
		// diff代码块测试 - 新增
		{
			name:     "diff代码块开始",
			input:    "```diff\n",
			expected: "#+begin_src diff\n",
		},
		// 空行测试
		{
			name:     "空行",
			input:    "\n",
			expected: "\n",
		},
	}

	converter := NewMarkdownToOrgConverter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重置转换器状态
			converter.inCodeBlock = false
			converter.currentCodeLang = ""

			result := converter.ConvertLine(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertLine() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMarkdownToOrgConverter_CodeBlock(t *testing.T) {
	converter := NewMarkdownToOrgConverter()

	// 测试代码块
	inputs := []string{
		"```python\n",
		"def hello():\n",
		"    print('Hello')\n",
		"```\n",
		"Normal text after code block\n",
	}

	expected := []string{
		"#+begin_src python\n",
		"def hello():\n",
		"    print('Hello')\n",
		"#+end_src\n",
		"Normal text after code block\n",
	}

	for i, input := range inputs {
		result := converter.ConvertLine(input)
		if result != expected[i] {
			t.Errorf("Code block test [%d]: ConvertLine() = %q, want %q", i, result, expected[i])
		}
	}

	// 验证转换器状态已重置
	if converter.inCodeBlock {
		t.Error("Converter should not be in code block after processing")
	}
}

func TestMarkdownToOrgConverter_DiffCodeBlock(t *testing.T) {
	converter := NewMarkdownToOrgConverter()

	// 测试diff代码块
	inputs := []string{
		"```diff\n",
		"--- a/file.go\n",
		"+++ b/file.go\n",
		"@@ -1,5 +1,5 @@\n",
		"-old line\n",
		"+new line\n",
		" context line\n",
		"```\n",
		"Normal text after diff block\n",
	}

	expected := []string{
		"#+begin_src diff\n",
		"--- a/file.go\n",
		"+++ b/file.go\n",
		"@@ -1,5 +1,5 @@\n",
		"-old line\n",
		"+new line\n",
		" context line\n",
		"#+end_src\n",
		"Normal text after diff block\n",
	}

	for i, input := range inputs {
		result := converter.ConvertLine(input)
		if result != expected[i] {
			t.Errorf("Diff code block test [%d]: ConvertLine() = %q, want %q", i, result, expected[i])
		}
	}

	// 验证转换器状态已重置
	if converter.inCodeBlock {
		t.Error("Converter should not be in code block after processing")
	}
}

func TestMarkdownToOrgConverter_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "只有空格",
			input:    "   \n",
			expected: "   \n",
		},
		{
			name:     "嵌套格式",
			input:    "**bold *italic* bold**\n",
			expected: "*bold /italic/ bold*\n",
		},
		{
			name:     "代码块无语言",
			input:    "```\n",
			expected: "#+begin_src text\n",
		},
		{
			name:     "代码块带语言",
			input:    "```javascript\n",
			expected: "#+begin_src javascript\n",
		},
		{
			name:     "diff代码块",
			input:    "```diff\n",
			expected: "#+begin_src diff\n",
		},
		{
			name:     "行尾无换行",
			input:    "No newline",
			expected: "No newline\n",
		},
	}

	converter := NewMarkdownToOrgConverter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter.inCodeBlock = false
			converter.currentCodeLang = ""

			result := converter.ConvertLine(tt.input)
			if result != tt.expected {
				t.Errorf("ConvertLine() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMarkdownToOrgConverter_BoldItalicOrder(t *testing.T) {
	converter := NewMarkdownToOrgConverter()

	// 测试粗体和斜体的顺序
	input := "**bold** and *italic* and **bold with *nested* italic**\n"
	expected := "*bold* and /italic/ and *bold with /nested/ italic*\n"

	result := converter.ConvertLine(input)
	if result != expected {
		t.Errorf("BoldItalicOrder: ConvertLine() = %q, want %q", result, expected)
	}
}

// TestMarkdownToOrgConverter_CodeBlockUnderscore 测试代码块中的下划线处理
func TestMarkdownToOrgConverter_CodeBlockUnderscore(t *testing.T) {
	converter := NewMarkdownToOrgConverter()

	// 测试1: Python代码块 - 下划线现在保持原样（不再插入零宽度空格）
	input1 := "```python\ndef test_function():\n    my_variable = \"test\"\n    another_var = 123\n    print(my_variable)\n```\n"

	lines1 := strings.Split(input1, "\n")
	var result1 strings.Builder
	for _, line := range lines1 {
		result1.WriteString(converter.ConvertLine(line + "\n"))
	}

	// 检查不包含零宽度空格
	output1 := result1.String()
	if strings.Contains(output1, "\u200b") {
		t.Error("Python代码块输出中不应该包含零宽度空格")
	}

	// 测试2: 普通文本中的下划线（保持不变）
	input2 := "This is normal_text with underscores.\n"
	expected2 := "This is normal_text with underscores.\n"
	result2 := converter.ConvertLine(input2)
	if result2 != expected2 {
		t.Errorf("普通文本下划线处理错误: got %q, want %q", result2, expected2)
	}

	// 测试3: 格式化文本中的下划线
	input3 := "**bold_text** and `code_with_underscores`\n"
	expected3 := "*bold_text* and =code_with_underscores=\n"
	result3 := converter.ConvertLine(input3)
	if result3 != expected3 {
		t.Errorf("格式化文本下划线处理错误: got %q, want %q", result3, expected3)
	}
}

// TestMarkdownToOrgConverter_UnderscoreInText 测试普通文本中的下划线
func TestMarkdownToOrgConverter_UnderscoreInText(t *testing.T) {
	converter := NewMarkdownToOrgConverter()

	// 普通文本中的下划线应该保持不变
	input := "This is normal_text with underscores_and_more.\n"
	expected := "This is normal_text with underscores_and_more.\n"

	result := converter.ConvertLine(input)
	if result != expected {
		t.Errorf("Underscore in text: ConvertLine() = %q, want %q", result, expected)
	}
}

func BenchmarkMarkdownToOrgConverter_ConvertLine(b *testing.B) {
	converter := NewMarkdownToOrgConverter()
	lines := []string{
		"# Benchmark Test\n",
		"## Section 1\n",
		"This is a **benchmark** test with *multiple* formats.\n",
		"Here's some `code` and a [link](http://example.com).\n",
		"```go\n",
		"func benchmark() {\n",
		"    // do something\n",
		"}\n",
		"```\n",
		"## Section 2\n",
		"More ~~text~~ to convert.\n",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		converter.inCodeBlock = false
		converter.currentCodeLang = ""
		for _, line := range lines {
			_ = converter.ConvertLine(line)
		}
	}
}

func TestMarkdownToOrgConverter_Table(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "basic markdown table",
			input: strings.Join([]string{
				"| Name  | Phone | Age |",
				"|-------|-------|-----|",
				"| Peter | 1234  | 17  |",
				"| Anna  | 4321  | 25  |",
				"",
			}, "\n"),
			expected: strings.Join([]string{
				"| Name  | Phone | Age |\n",
				"|-------+-------+-----|\n",
				"| Peter | 1234  | 17  |\n",
				"| Anna  | 4321  | 25  |\n",
				"\n",
			}, ""),
		},
		{
			name: "markdown table with inline formatting",
			input: strings.Join([]string{
				"| **Bold** | *Italic* | `code` |",
				"|----------|----------|--------|",
				"| ~~strike~~ | <u>under</u> | text |",
				"",
			}, "\n"),
			expected: strings.Join([]string{
				"| *Bold*   | /Italic/     | =code= |\n",
				"|----------+--------------+--------|\n",
				"| +strike+ | <u>under</u> | text   |\n",
				"\n",
			}, ""),
		},
		{
			name: "markdown table with links",
			input: strings.Join([]string{
				"| Site | URL |",
				"|------|-----|",
				"| [Go](https://go.dev) | [Example](https://example.com) |",
				"",
			}, "\n"),
			expected: strings.Join([]string{
				"| Site                   | URL                              |\n",
				"|------------------------+----------------------------------|\n",
				"| [[https://go.dev][Go]] | [[https://example.com][Example]] |\n",
				"\n",
			}, ""),
		},
		{
			name: "markdown table inside code block - not converted",
			input: strings.Join([]string{
				"```text",
				"| Name | Value |",
				"|------|-------|",
				"| foo  | bar   |",
				"```",
				"",
			}, "\n"),
			expected: strings.Join([]string{
				"#+begin_src text\n",
				"| Name | Value |\n",
				"|------|-------|\n",
				"| foo  | bar   |\n",
				"#+end_src\n",
				"\n",
			}, ""),
		},
		{
			name: "markdown table with alignment syntax",
			input: strings.Join([]string{
				"| Left | Center | Right |",
				"|:-----|:------:|------:|",
				"| L1   | C1     | R1    |",
				"",
			}, "\n"),
			expected: strings.Join([]string{
				"| Left | Center | Right |\n",
				"|------+--------+-------|\n",
				"| L1   | C1     | R1    |\n",
				"\n",
			}, ""),
		},
		{
			name: "not a table - missing separator",
			input: strings.Join([]string{
				"| Name | Phone |",
				"| Peter | 1234 |",
				"",
			}, "\n"),
			expected: strings.Join([]string{
				"| Name | Phone |\n",
				"| Peter | 1234 |\n",
				"\n",
			}, ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToOrgConverter()

			var result strings.Builder
			lines := strings.Split(tt.input, "\n")
			for _, line := range lines {
				result.WriteString(converter.ConvertLine(line + "\n"))
			}
			// Flush any remaining table buffer
			if len(converter.tableBuf) > 0 {
				result.WriteString(converter.flushMdTableBuf())
			}

			got := result.String()
			if got != tt.expected {
				t.Errorf("Table conversion:\n got:  %q\n want: %q", got, tt.expected)
			}
		})
	}
}
