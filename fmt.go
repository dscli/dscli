package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	// 当前输出模式
	OutputMode string = "markdown"

	// 输出写入器
	outputWriter io.Writer = os.Stdout
)

// Println 根据当前模式输出
func Println(a ...any) (n int, err error) {
	text := fmt.Sprint(a...)
	if OutputMode == "org" {
		text = convertMarkdownToOrg(text)
	}
	return fmt.Fprintln(outputWriter, text)
}

// Printf 根据当前模式输出
func Printf(format string, a ...any) (n int, err error) {
	text := fmt.Sprintf(format, a...)
	if OutputMode == "org" {
		text = convertMarkdownToOrg(text)
	}
	return fmt.Fprint(outputWriter, text)
}

// SetOutputMode 设置输出模式
func SetOutputMode(mode string) {
	OutputMode = mode
}

// SetOutputWriter 设置输出写入器
func SetOutputWriter(w io.Writer) {
	outputWriter = w
}

// convertMarkdownToOrg 将Markdown转换为Org模式
func convertMarkdownToOrg(text string) string {
	// 使用现有的MarkdownToOrgConverter
	converter := NewMarkdownToOrgConverter()

	// 由于ConvertLine需要处理换行符，我们按行处理
	lines := strings.Split(text, "\n")
	var result strings.Builder

	for i, line := range lines {
		// 添加换行符，因为ConvertLine期望有换行符
		converted := converter.ConvertLine(line + "\n")
		// 移除ConvertLine添加的额外换行符
		converted = strings.TrimSuffix(converted, "\n")
		result.WriteString(converted)

		// 如果不是最后一行，添加换行符
		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}
