package file

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "search_files",
		Description: "在项目目录中搜索文件，支持文件名模式匹配（如*.go）和文件内容搜索。自动排除.git目录。",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "文件名模式，如 '*.go'，为空则匹配所有文件",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要搜索的内容（如果提供则搜索文件内容）,长度1-4096字符",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleSearchFiles,
	})
}

// handleSearchFiles 搜索文件
func handleSearchFiles(ctx context.Context, args ToolArgs) (output string, user string, err error) {
	pattern := toolcall.ToolArgsValue(args, "pattern", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	// 使用find和grep命令实现搜索
	// 基础find命令：从当前目录开始，排除.git目录，只搜索文件
	script := `find . -type f -not -path "./.git/*"`

	// 添加文件名模式匹配
	if pattern != "" {
		// 将Go的glob模式转换为find的-name模式
		// 注意：这里简化处理，复杂的glob模式可能需要转换
		// 转义单引号：将'替换为'\''
		escapedPattern := strings.ReplaceAll(pattern, "'", "'\"'\"'")
		script += fmt.Sprintf(` -name '%s'`, escapedPattern)
	}

	// 添加内容匹配
	if content != "" {
		// 使用-exec和grep进行内容搜索
		// -l: 只显示包含匹配内容的文件名
		// -q: 安静模式，只返回退出状态
		// 转义单引号：将'替换为'\''
		escapedContent := strings.ReplaceAll(content, "'", "'\"'\"'")
		script += fmt.Sprintf(` -exec grep -lq '%s' {} \;`, escapedContent)
	}

	// 输出结果并限制数量
	script += ` -print 2>/dev/null | head -50`

	// 处理空结果
	script += ` || echo "未找到匹配的文件"`

	output, err = toolcall.RunShell(ctx, script)
	return
}
