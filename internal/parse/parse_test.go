package parse

import (
	"crypto/md5"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestParseMarkdownFileStructure(t *testing.T) {
	// 测试Markdown文件解析
	content := `# 测试Markdown文件

这是一个测试Markdown文件，用于测试解析功能。

## 二级标题

这是一个二级标题的内容。

### 三级标题

这是一个三级标题的内容。

` + "```" + `go
package main

func main() {
    println("Hello, World!")
}
` + "```" + `

## 另一个二级标题

- 列表项1
- 列表项2
- 列表项3

[链接到Google](https://www.google.com)

**粗体文本** *斜体文本*

> 引用块内容
`
	err := os.WriteFile("test.md", []byte(content), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove("test.md")
	fs, err := ParseFileStructure(t.Context(), "test.md")
	if err != nil {
		t.Fatalf("ParseFileStructure失败: %v", err)
	}

	// 验证语言识别
	if fs.Language != "markdown" {
		t.Errorf("期望语言为markdown，实际为: %s", fs.Language)
	}

	// 验证文件路径
	if fs.FilePath != "test.md" {
		t.Errorf("期望文件路径为test.md，实际为: %s", fs.FilePath)
	}

	// 验证标题解析
	expectedHeadings := 4 // #, ##, ###, ##
	if len(fs.Classes) != expectedHeadings {
		t.Errorf("期望%d个标题，实际有%d个", expectedHeadings, len(fs.Classes))
	}

	// 验证代码块解析 - 包括Go代码块和链接
	if len(fs.Functions) != 2 {
		t.Errorf("期望2个代码块（1个Go代码块 + 1个链接），实际有%d个", len(fs.Functions))
	}

	// 验证列表项解析
	if len(fs.Imports) != 3 {
		t.Errorf("期望3个列表项，实际有%d个", len(fs.Imports))
	}

	// 验证第一个标题
	if len(fs.Classes) > 0 {
		firstHeading := fs.Classes[0]
		if firstHeading.Name != "测试Markdown文件" {
			t.Errorf("第一个标题期望为'测试Markdown文件'，实际为: %s", firstHeading.Name)
		}
		if firstHeading.Type != "heading_1" {
			t.Errorf("第一个标题类型期望为'heading_1'，实际为: %s", firstHeading.Type)
		}
		if firstHeading.Line != 1 {
			t.Errorf("第一个标题行号期望为1，实际为: %d", firstHeading.Line)
		}
	}

	// 验证代码块 - 需要找到Go代码块
	var goCodeBlock *Symbol
	for _, f := range fs.Functions {
		if f.Type == "code_block" {
			goCodeBlock = f
			break
		}
	}

	if goCodeBlock != nil {
		// 代码块名称是自动生成的，如"code_block_13"
		if !strings.Contains(goCodeBlock.Name, "code_block") {
			t.Errorf("代码块名称期望包含'code_block'，实际为: %s", goCodeBlock.Name)
		}
		if goCodeBlock.Type != "code_block" {
			t.Errorf("代码块类型期望为'code_block'，实际为: %s", goCodeBlock.Type)
		}
	} else {
		t.Errorf("未找到Go代码块")
	}
}

func TestPythonScript(t *testing.T) {
	hash1 := fmt.Sprintf("%x", md5.Sum([]byte(pythonScript)))
	t.Log(hash1)
	script := `#!/usr/bin/env python
print("OK")`
	hash2 := fmt.Sprintf("%x", md5.Sum([]byte(script)))
	t.Log(hash2)
}

func TestSymbolFromMap(t *testing.T) {
	// 完整字段
	m := map[string]any{
		"name":       "TestFunc",
		"type":       "function",
		"lineno":     float64(10),
		"end_lineno": float64(25),
	}
	s := symbolFromMap(m)
	if s.Name != "TestFunc" {
		t.Errorf("Name = %q, want %q", s.Name, "TestFunc")
	}
	if s.Type != "function" {
		t.Errorf("Type = %q, want %q", s.Type, "function")
	}
	if s.Line != 10 {
		t.Errorf("Line = %d, want 10", s.Line)
	}
	if s.EndLine != 25 {
		t.Errorf("EndLine = %d, want 25", s.EndLine)
	}

	// 无end_lineno时应默认为Line
	m2 := map[string]any{
		"name":   "NoEnd",
		"type":   "class",
		"lineno": float64(5),
	}
	s2 := symbolFromMap(m2)
	if s2.EndLine != 5 {
		t.Errorf("EndLine = %d, want 5 (default to Line)", s2.EndLine)
	}

	// 无lineno时两者均为0
	m3 := map[string]any{
		"name": "NoLine",
		"type": "variable",
	}
	s3 := symbolFromMap(m3)
	if s3.Line != 0 || s3.EndLine != 0 {
		t.Errorf("Line=%d, EndLine=%d, want both 0", s3.Line, s3.EndLine)
	}
}

func TestExtractSymbols(t *testing.T) {
	result := map[string]any{
		"funcs": []any{
			map[string]any{"name": "f1", "type": "function", "lineno": float64(1)},
			map[string]any{"name": "f2", "type": "function", "lineno": float64(5), "end_lineno": float64(10)},
		},
	}
	symbols := extractSymbols(result, "funcs")
	if len(symbols) != 2 {
		t.Fatalf("len = %d, want 2", len(symbols))
	}
	if symbols[0].Line != 1 || symbols[0].EndLine != 1 {
		t.Errorf("f1: Line=%d, EndLine=%d; want 1,1", symbols[0].Line, symbols[0].EndLine)
	}
	if symbols[1].Line != 5 || symbols[1].EndLine != 10 {
		t.Errorf("f2: Line=%d, EndLine=%d; want 5,10", symbols[1].Line, symbols[1].EndLine)
	}

	// key不存在时返回nil
	if symbols := extractSymbols(result, "nonexistent"); symbols != nil {
		t.Errorf("expected nil for nonexistent key, got %v", symbols)
	}
}

func TestExtractStrings(t *testing.T) {
	result := map[string]any{
		"imports": []any{"fmt", "os", "strings"},
		"mixed":   []any{"a", 123, "b"}, // 非string元素被跳过
	}
	strs := extractStrings(result, "imports")
	if len(strs) != 3 {
		t.Errorf("len = %d, want 3", len(strs))
	}
	strs2 := extractStrings(result, "mixed")
	if len(strs2) != 2 {
		t.Errorf("len = %d, want 2 (non-strings skipped)", len(strs2))
	}
	if strs := extractStrings(result, "nonexistent"); strs != nil {
		t.Errorf("expected nil, got %v", strs)
	}
}

func TestExtractNames(t *testing.T) {
	result := map[string]any{
		"lists": []any{
			map[string]any{"name": "item1"},
			map[string]any{"name": ""}, // 空字符串跳过
			map[string]any{"name": "item3"},
			map[string]any{"other": "value"}, // 无name字段跳过
		},
	}
	names := extractNames(result, "lists")
	if len(names) != 2 {
		t.Fatalf("len = %d, want 2", len(names))
	}
	if names[0] != "item1" || names[1] != "item3" {
		t.Errorf("names = %v, want [item1 item3]", names)
	}
}

func TestAppendFormatted(t *testing.T) {
	result := map[string]any{
		"vars": []any{
			map[string]any{"name": "a", "type": "str"},
			map[string]any{"name": "b", "type": "int"},
		},
	}
	var out []string
	appendFormatted(result, "vars", &out, "%s (%s)", "name", "type")
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2", len(out))
	}
	if out[0] != "a (str)" || out[1] != "b (int)" {
		t.Errorf("out = %v, want [a (str) b (int)]", out)
	}

	// 单key映射
	var out2 []string
	appendFormatted(result, "vars", &out2, "mapping: %s", "name")
	if out2[0] != "mapping: a" || out2[1] != "mapping: b" {
		t.Errorf("out2 = %v", out2)
	}
}
