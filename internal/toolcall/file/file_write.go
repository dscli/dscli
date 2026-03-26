package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "write_file",
		Description: `将内容写入文件。重要：如果内容超过 8192 字符，你必须分多次调用，每次写入小于 8192 长度内容，
首次使用 append=false，后续使用 append=true 追加。注意 append 默认值 true，默认支持追加。支持自动创建目录结构。
示例：若文件有 20000 字符，应分三次调用：
1. append=false, content="第一部分(≤8192字符)"
2. append=true, content="第二部分(≤8192字符)"
3. append=true, content="剩余部分(≤8192字符)"
`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径",
					"pattern":     toolcall.TitleLikePattern(128),
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "是否追加，false 覆盖或创建，true 在文件末尾追加, 默认为true",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "写入的内容",
					"pattern":     toolcall.ContentLikePattern(8192),
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})
}

// handleWriteFile 写入文件
func handleWriteFile(ctx context.Context, args toolcall.ToolArgs) (result string, user string, err error) {
	truncated := context.ContextValue(ctx, context.FinishReasonLengthKey, false)
	outfmt.Printf("FinishReasonLength: %v\n", truncated)
	if truncated {
		outfmt.Println("内容截断，在恢复")
		err = args.Unmarshal()
		if err != nil {
			err = fmt.Errorf("failed to unmarshal from raw args")
			outfmt.Println("内容截断，恢复失败")
			return
		}
		outfmt.Println("内容截断，已经恢复")
	}

	path := toolcall.ToolArgsValue(args, "path", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	lastlines := ""
	if truncated && len(content) > 100 {
		runes := []rune(content)
		lastlines = string(runes[len(runes)-100:])
		outfmt.Printf("最后几行：\n%s\n", lastlines)
	}

	if path == "" || content == "" {
		if truncated {
			user = fmt.Sprintf(`内容超过 max_tokens=8192, 已经截断。其中path参数也被截断,
因此截断内容虽然收到但因不知写入哪个文件，最终无法写入。
请严格按照 write_file 的使用说明，严控输出长度不超 8192 Tokens，
在输出内容(content)前，优先考虑输出到哪个文件（path 参数），
注意content无法写入已丢弃，请重新生成这部分content，
以下将已输出content内容的最后几行也附上供参考:

%s`, lastlines)
		}
		err = fmt.Errorf("文件路径 path 或文件内容 content 不能为空")
		return
	}

	append := toolcall.ToolArgsValue(args, "append", true)

	fullPath := ResolvePath(ctx, path)
	dirPath := filepath.Dir(fullPath)
	var fi os.FileInfo
	fi, err = os.Stat(dirPath)
	if err == nil && !fi.IsDir() {
		err = fmt.Errorf("%s is not directory", dirPath)
		return
	}

	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, 0o755)
	}

	if err != nil {
		err = fmt.Errorf("failed to get or create directory %s: %w", dirPath, err)
		return
	}

	var file *os.File
	if append {
		file, err = os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	} else {
		file, err = os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	}
	if err != nil {
		err = fmt.Errorf("无法打开文件: %w", err)
		return
	}
	defer file.Close()

	// 检查文件是否为空
	fi, err = os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("获取文件信息失败: %w", err)
		return
	}

	// 如果文件存在且不为空，先添加换行符（除非追加内容以换行符开头）
	if err == nil && fi.Size() > 0 {
		// 检查追加内容是否以换行符开头
		if !strings.HasPrefix(content, "\n") {
			// 总是添加换行符，确保追加内容在新行
			if _, err = file.WriteString("\n"); err != nil {
				err = fmt.Errorf("写入换行符失败: %w", err)
				return
			}
		}
	}

	// 写入内容
	if _, err = file.WriteString(content); err != nil {
		err = fmt.Errorf("写入内容失败: %w", err)
		return
	}

	lines := strings.Count(content, "\n") + 1
	if content == "" || strings.HasSuffix(content, "\n") {
		lines = strings.Count(content, "\n")
	}

	outfmt.Notice("追加内容到文件 \"%s\"，添加 %d 行", path, lines)

	// 构建最终结果
	result = fmt.Sprintf("成功追加内容到文件 \"%s\"，添加 %d 行。", path, lines)

	if truncated {
		user = fmt.Sprintf(`此次写入文件 %s 的内容是截断的内容，其中最后一行因为不完整已丢弃，
请从上次输出内容的最后一完整行继续生成，并调用工具 write_file(path="%s", append=true, content="...继续生成的内容...") 
追加入文件%s，为帮助你找到继续生成的点，现把上次截断内容最后几行展示给你：
---
%s---
如果觉得信息不足以继续生成，可以停下来询问。`, path, path, path, lastlines)
	} else {
		n := utf8.RuneCountInString(content)
		if n > 8192 {
			user = fmt.Sprintf(`内容成功写入文件，但这部分内容太大(%d > 8192)，请严格按照 write_file 工具要求，严格控制输出长度。`, n)
		}
	}
	return
}
