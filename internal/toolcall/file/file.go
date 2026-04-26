// Package file provides file ops tool calls
package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

type (
	ToolArgs = toolcall.ToolArgs
)


// ResolvePath 解析文件路径：
//  1. 如果是 "~" 开头（如 ~/.dscli/skills/...），展开为用户主目录
//  2. 如果是绝对路径，直接使用
//  3. 否则是相对路径，拼接项目根目录
func ResolvePath(ctx context.Context, path string) string {
	// Unix ~ 展开：~/.dscli/... → /home/user/.dscli/...
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[1:])
		}
	}
	projectRoot := context.ProjectRoot
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectRoot, path)
}

// ParseLineRange parse line range
func ParseLineRange(args ToolArgs) (int, int, error) {
	// 解析开始行号
	startLine := int(toolcall.ToolArgsValue(args, "start_line", int64(1)))
	// 解析结束行号
	endLine := int(toolcall.ToolArgsValue(args, "end_line", int64(-1))) // -1 表示到文件末尾
	// 验证行号范围
	if endLine != -1 && endLine < startLine {
		return 0, 0, fmt.Errorf("end_line must be greater than or equal to start_line")
	}
	return startLine, endLine, nil
}