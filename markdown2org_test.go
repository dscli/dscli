package main

import (
	"bytes"
	"fmt"
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
			expected: "#+begin_src \n",
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

func TestMarkdownToOrgConverter_ConvertStream(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "完整文档转换",
			input: `# Main Title

This is a **bold** statement with *italic* text.

## Subsection

Here's some ` + "`inline code`" + ` and a [link](https://example.com).

### Code Example

` + "```" + `go
package main

func main() {
    fmt.Println("Hello")
}
` + "```" + `

More text after code.
`,
			expected: `* Main Title

This is a *bold* statement with /italic/ text.

** Subsection

Here's some =inline code= and a [[https://example.com][link]].

*** Code Example

#+begin_src go
package main

func main() {
    fmt.Println("Hello")
}
#+end_src

More text after code.
`,
		},
		{
			name:     "流式输入",
			input:    "Line 1\nLine 2\nLine 3\n",
			expected: "Line 1\nLine 2\nLine 3\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			converter := NewMarkdownToOrgConverter()
			input := strings.NewReader(tt.input)
			var output bytes.Buffer

			err := converter.ConvertStream(input, &output)
			if err != nil {
				t.Fatalf("ConvertStream() error = %v", err)
			}

			result := output.String()
			if result != tt.expected {
				t.Errorf("ConvertStream() = %q, want %q", result, tt.expected)
			}
		})
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
			expected: "#+begin_src \n",
		},
		{
			name:     "代码块带语言",
			input:    "```javascript\n",
			expected: "#+begin_src javascript\n",
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

func BenchmarkMarkdownToOrgConverter_ConvertStream(b *testing.B) {
	converter := NewMarkdownToOrgConverter()
	
	// 创建测试数据
	var builder strings.Builder
	for i := 0; i < 1000; i++ {
		builder.WriteString(fmt.Sprintf("# Heading %d\n", i))
		builder.WriteString(fmt.Sprintf("This is line %d with **bold** text.\n", i))
		if i%10 == 0 {
			builder.WriteString("```go\nfunc test() {}\n```\n")
		}
	}
	testData := builder.String()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		input := strings.NewReader(testData)
		var output bytes.Buffer
		_ = converter.ConvertStream(input, &output)
	}
}
