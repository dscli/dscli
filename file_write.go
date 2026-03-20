package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name: "write_file",
		Description: `将内容写入文件，如果文件不存在则创建，如果文件存在，append=false 覆盖，append=true 追加。
支持创建目录结构。`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
					"pattern":     TitleLikePattern(128),
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要新建或追加的内容，建议不超过4096个字符",
					"pattern":     ContentLikePattern(4096),
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "是否以追加模式写入，默认为 false",
				},
			},
			"required":             []string{"path", "content", "append"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})
}

// handleWriteFile 写入文件
func handleWriteFile(ctx context.Context, args ToolArgs) (output string, err error) {
	path := ToolArgsValue(args, "path", "")
	content := ToolArgsValue(args, "content", "")
	if path == "" || content == "" {
		err = fmt.Errorf("文件路径 path 和文件内容 content 都不能为空")
		return
	}

	append := ToolArgsValue(args, "append", false)
	delete(args, "append")

	fullPath := resolvePath(ctx, path)

	if append {
		// 追加模式：打开文件并追加内容
		file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return "", fmt.Errorf("无法打开文件进行追加: %w", err)
		}
		defer file.Close()

		// 检查文件是否为空
		fi, err := os.Stat(fullPath)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("获取文件信息失败: %w", err)
		}

		// 如果文件存在且不为空，先添加换行符（除非追加内容以换行符开头）
		if err == nil && fi.Size() > 0 {
			// 检查追加内容是否以换行符开头
			if !strings.HasPrefix(content, "\n") {
				// 总是添加换行符，确保追加内容在新行
				if _, err := file.WriteString("\n"); err != nil {
					return "", fmt.Errorf("写入换行符失败: %w", err)
				}
			}
		}

		// 写入内容
		if _, err := file.WriteString(content); err != nil {
			return "", fmt.Errorf("写入内容失败: %w", err)
		}

		lines := strings.Count(content, "\n") + 1
		if content == "" || strings.HasSuffix(content, "\n") {
			lines = strings.Count(content, "\n")
		}

		Notice("追加内容到文件 \"%s\"，添加 %d 行", path, lines)

		// 运行make format并捕获结果
		formatOutput, formatErr := CodeMakeFormat(ctx, filepath.Ext(path))

		// 构建最终结果
		result := fmt.Sprintf("成功追加内容到文件 \"%s\"，添加 %d 行", path, lines)

		// 添加格式化结果信息
		if formatErr != nil {
			result += fmt.Sprintf("\n⚠️ 代码格式化失败: %v", formatErr)
		} else if formatOutput != "" {
			// 格式化成功且有输出
			formatOutput = strings.TrimSpace(formatOutput)
			result += fmt.Sprintf("\n✅ 代码格式化完成: %s", formatOutput)
		}
		// 注意：如果formatOutput为空且formatErr为nil，表示不需要格式化
		// 这种情况下不添加任何格式化相关的消息

		return result, nil
	}

	// 非追加模式：使用现有的行范围写入
	return handleWriteFileWithLineRange(ctx, args)
}
