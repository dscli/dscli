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
