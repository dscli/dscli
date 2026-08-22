package file

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/flycheck"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed file_write_with_line_range.md
var file_write_with_line_range_md string

func init() {
	// 注册文件行范围写入工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "write_file_with_line_range",
		Description: file_write_with_line_range_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, e.g. main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write; empty string to delete lines, max 4096 chars recommended",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "Start line (1-based), optional, default 1",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "End line (inclusive). Optional: defaults to start_line (single-line edit); use -1 to replace to end of file.",
				},
				"insert_before_line": map[string]any{
					"type":        "integer",
					"description": "Insert content BEFORE this 1-based line; original line N and below shift down. Mutually exclusive with start_line/end_line.",
				},
				"line_tag": map[string]any{
					"type":        "string",
					"description": "4-char CAS tag for start_line (single-line edit). If provided, verified before write.",
				},
				"line_tags": map[string]any{
					"type":        "string",
					"description": "Newline-separated 4-char CAS tags, one per line in the range. Verified before write.",
				},
				"context": map[string]any{
					"type":        "boolean",
					"description": "After editing, return a context window around the edit. Default true. Set false to suppress.",
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFileWithLineRange,
	})
}

// handleWriteFileWithLineRange 写入文件指定行范围的内容
// 如果 content 为空字符串，则删除指定行范围
// 支持 CAS tag 校验：line_tag（单行）或 line_tags（多行）用于防竞态写入
func handleWriteFileWithLineRange(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleWriteFileWithLineRange")
	defer span.Finish()
	// 检查必需参数
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("parameter error: no path specified")
		return result, warning, err
	}

	content := toolcall.ToolArgsValue(args, "content", "")
	showContext := toolcall.ToolArgsValue(args, "context", true)

	// CAS tag 污染检测：write_file_with_line_range 的 content 参数也不应含有 CAS tag
	if n := detectCASTags(content); n >= casTagThreshold {
		err = fmt.Errorf(
			"内容包含疑似 read_file CAS tag（检测到 %d 行含有 CAS tag 前缀）。\n"+
				"write_file_with_line_range 的 content 参数不应包含 read_file 输出的行首 TAG（如 \"[Q8fA]\"）。\n"+
				"请去除这些 CAS tag 前缀后重试。",
			n,
		)
		return result, warning, err
	}

	// content 可以为空字符串，表示删除

	fullPath := ResolvePath(ctx, path)
	displayPath := DisplayPath(ctx, fullPath)

	// 插入模式：insert_before_line 与替换路径完全分叉，
	// 避免 ParseLineRange 的默认行范围（1..EOF）干扰插入语义。
	// 互斥校验集中在分叉处：插入点只有一个，替换类参数在此模式下语义冲突。
	if insertLine, ok := parseInsertBeforeLine(args); ok {
		for _, k := range []string{"start_line", "end_line", "line_tags"} {
			if _, has := args[k]; has {
				err = fmt.Errorf("insert_before_line cannot be combined with %s", k)
				return result, warning, err
			}
		}
		return handleInsertBeforeLine(ctx, args, fullPath, displayPath, content, insertLine, showContext)
	}

	// 解析起始行号
	startLine, endLine, err := ParseLineRange(args)
	if err != nil {
		err = fmt.Errorf("failed to parse line range: %w", err)
		return result, warning, err
	}

	// 方案A: 不指定 end_line 时默认只编辑 start_line 这一行
	// 如需替换到文件末尾，请显式传递 end_line=-1
	if _, endLineProvided := args["end_line"]; !endLineProvided {
		endLine = startLine
	}

	// 计算新内容的行数
	contentLineCount := strings.Count(content, "\n") + 1
	if content == "" || strings.HasSuffix(content, "\n") {
		contentLineCount = strings.Count(content, "\n")
	}

	// 读取原文件所有行
	file, err := os.Open(fullPath)
	if err != nil {
		// 如果文件不存在，创建一个空文件
		if os.IsNotExist(err) {
			// 对于新文件，只能从第1行开始写入
			if startLine != 1 {
				err = fmt.Errorf("cannot write to non-existent file at line %d, must start from line 1", startLine)
				return result, warning, err
			}

			// 创建新文件并写入内容
			if content == "" {
				// 空内容，创建空文件
				var newFile *os.File
				newFile, err = os.Create(fullPath)
				if err != nil {
					err = fmt.Errorf("failed to create file: %w", err)
					return result, warning, err
				}
				newFile.Close()
				outfmt.Notice("创建空文件 \"%s\"", displayPath)
				result = "成功创建空文件"
				return result, warning, err
			}

			// 写入内容到新文件，确保末尾换行
			writeContent := content
			if writeContent != "" && !strings.HasSuffix(writeContent, "\n") {
				writeContent += "\n"
			}
			err = os.WriteFile(fullPath, []byte(writeContent), 0o644)
			if err != nil {
				err = fmt.Errorf("failed to write to new file: %w", err)
				return result, warning, err
			}

			outfmt.Notice("创建文件 \"%s\" 并写入 %d 行内容", displayPath, contentLineCount)
			result = fmt.Sprintf("成功创建文件并写入 %d 行内容", contentLineCount)

			// 上下文窗口（新文件）
			if showContext {
				ctxStr := AppendWriteFileContext(displayPath)
				if ctxStr != "" {
					result += ctxStr
				}
			}
			return result, warning, err
		}
		err = fmt.Errorf("failed to open file: %w", err)
		return result, warning, err
	}
	defer file.Close()

	// 读取所有行
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		err = fmt.Errorf("failed to read file: %w", err)
		return result, warning, err
	}

	oldTotalLines := len(lines)

	// --- CAS tag verification (antirez-style check-and-set) ---
	// 如果提供了 line_tag 或 line_tags，写入前校验标签匹配
	lineTag := toolcall.ToolArgsValue(args, "line_tag", "")
	lineTags := toolcall.ToolArgsValue(args, "line_tags", "")
	var expectedTags []string

	if lineTag != "" || lineTags != "" {
		if lineTag != "" && lineTags != "" {
			err = fmt.Errorf("cannot specify both line_tag and line_tags; use line_tag for single-line edits, line_tags for multi-line")
			return result, warning, err
		}
		if lineTag != "" {
			if len(lineTag) != 4 {
				err = fmt.Errorf("line_tag must be exactly 4 characters, got %q (%d chars)", lineTag, len(lineTag))
				return result, warning, err
			}
			expectedTags = []string{lineTag}
		} else {
			expectedTags, err = parseLineTags(lineTags)
			if err != nil {
				err = fmt.Errorf("failed to parse line_tags: %w", err)
				return result, warning, err
			}
		}

		// Verify tags against actual file content at startLine
		if err = verifyLineTags(lines, startLine-1, expectedTags); err != nil {
			return result, warning, err
		}
	}

	// Auto-strip CAS tag prefixes from content, using verified tags.
	// This handles the common case where the LLM includes read_file
	// CAS tag prefixes (like "Q8fA" or "[Q8fA]") in the content parameter.
	// Safe because we only strip prefixes matching the verified tags.
	if len(expectedTags) > 0 {
		stripped, didStrip := stripCASTags(content, expectedTags)
		if didStrip {
			content = stripped
			warning = "注意：已自动去除 content 中的 CAS tag 前缀（匹配已验证的 tag）。"
		}
	}

	// --- 构建新内容 ---
	var newLines []string

	// 1. 添加 start_line 之前的部分
	beforeStart := min(startLine-1, len(lines))
	if beforeStart > 0 {
		newLines = append(newLines, lines[:beforeStart]...)
	}
	// 2. 如果 startLine 超出文件范围，需要插入空行
	if startLine > len(lines) {
		emptyLinesNeeded := startLine - len(lines) - 1
		for range emptyLinesNeeded {
			newLines = append(newLines, "")
		}
	}

	// 3. 处理新内容
	if content != "" {
		// 分割新内容为多行。去掉末尾换行符，避免 Split 产生多余空行
		// （"a\nb\n" → ["a","b",""] 会在行中间插入空行）。
		contentLines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
		newLines = append(newLines, contentLines...)
	}
	// 如果 content 为空，这里什么都不添加，相当于删除

	// 4. 添加 end_line 之后的部分
	if endLine != -1 {
		// endLine 是包含的结束行号，所以之后的部分从 endLine 开始
		// 但需要确保 endLine 在文件范围内
		if endLine < len(lines) {
			newLines = append(newLines, lines[endLine:]...)
		}
	}

	// 将新内容写回文件，确保末尾有换行符
	var contentBuilder strings.Builder
	for i, line := range newLines {
		contentBuilder.WriteString(line)
		if i < len(newLines)-1 {
			contentBuilder.WriteString("\n")
		}
	}
	// POSIX 约定：文本文件应以换行符结尾
	writeContent := contentBuilder.String()
	if writeContent != "" && !strings.HasSuffix(writeContent, "\n") {
		writeContent += "\n"
	}
	err = os.WriteFile(fullPath, []byte(writeContent), 0o644)
	if err != nil {
		err = fmt.Errorf("failed to write file: %w", err)
		return result, warning, err
	}

	// 记录操作日志
	operation := "替换"
	if content == "" {
		operation = "删除"
	}

	rangeDesc := fmt.Sprintf("第%d行 - 第%d行", startLine, endLine)
	if endLine == -1 {
		rangeDesc = fmt.Sprintf("第%d行 - 末尾", startLine)
	}

	// 计算被替换的原始行数
	oldReplaced := 0
	if endLine == -1 {
		oldReplaced = max(0, oldTotalLines-startLine+1)
	} else {
		oldReplaced = max(0, min(endLine, oldTotalLines)-startLine+1)
	}
	linesChanged := oldReplaced
	if content != "" {
		linesChanged = contentLineCount
	}

	// 长度突变告警（无 CAS tag 时）：替换区域行数与写入内容行数严重不匹配
	// 是"行号错位覆盖"的典型信号——AI 本意在某处插入新内容，但目标区域实际
	// 行数远少于内容行数，静默覆盖了不该覆盖的内容。仅告警不阻断：合法的大
	// 规模替换（如展开函数体）也会触发，由 LLM 自行判断。
	if content != "" && endLine != -1 && lineTag == "" && lineTags == "" {
		if m := warnOnLengthMismatch(oldReplaced, contentLineCount); m != "" {
			if warning != "" {
				warning += "\n\n"
			}
			warning += m
		}
	}

	outfmt.Notice("%s文件 \"%s\" 行范围 %s，影响 %d 行", operation, displayPath, rangeDesc, linesChanged)

	// 构建最终结果
	result = fmt.Sprintf("成功%s文件 \"%s\" 行范围 %s", operation, displayPath, rangeDesc)

	// 编辑后上下文窗口
	if showContext {
		effectiveEndLine := endLine
		if effectiveEndLine == -1 {
			effectiveEndLine = oldTotalLines
		}
		if effectiveEndLine > oldTotalLines {
			effectiveEndLine = oldTotalLines
		}
		ctxStr := AppendEditContext(displayPath, startLine, effectiveEndLine, oldReplaced, contentLineCount)
		if ctxStr != "" {
			result += ctxStr
		}
	}

	// Run flycheck on the written file and append issues as suggestion
	if flyResult, _, flyErr := flycheck.Flycheck(ctx, fullPath); flyErr == nil && flyResult != "" {
		if warning != "" {
			warning += "\n\n"
		}
		warning += flyResult
	}

	return result, warning, err
}

// warnOnLengthMismatch 检测替换区域行数与写入内容行数的严重不匹配。
// 条件：内容行数 ≥ 原区域行数 + 10 且 ≥ 3 倍原区域行数（"插入意图"特征）。
// 返回告警文案；不匹配不严重时返回空字符串。
func warnOnLengthMismatch(oldReplaced, contentLineCount int) string {
	if oldReplaced < 1 || contentLineCount < oldReplaced+10 || contentLineCount < 3*oldReplaced {
		return ""
	}
	return fmt.Sprintf(
		"⚠️ 注意：替换区域仅 %d 行，但写入内容有 %d 行。若本意是插入新内容而非覆盖，"+
			"请确认行号未错位；或使用 read_file 获取 line_tag/line_tags CAS 校验后再写入。",
		oldReplaced, contentLineCount,
	)
}

// parseInsertBeforeLine 读取 insert_before_line 参数。
// 仅当参数显式提供时返回插入模式（ok=true），insertLine 为 1-based 插入点。
func parseInsertBeforeLine(args ToolArgs) (insertLine int, ok bool) {
	_, ok = args["insert_before_line"]
	if !ok {
		return 0, false
	}
	return int(toolcall.ToolArgsValue(args, "insert_before_line", int64(0))), true
}

// handleInsertBeforeLine 实现 insert_before_line 插入语义：
// content 插入到第 N 行之前，原第 N 行及之后顺延（N=len+1 表示追加到末尾）。
//
// 与替换路径（start_line/end_line）完全独立：替换路径的默认行范围
// （start=1, end=EOF）会让"插入"退化为"覆盖全文件"，这是本参数存在的意义。
// line_tag 校验插入点行（追加末尾时校验最后一行），防止文件已变更时插错位置。
func handleInsertBeforeLine(ctx context.Context, args ToolArgs, fullPath, displayPath, content string, insertLine int, showContext bool) (result, warning string, err error) {
	// 注：与 start_line/end_line/line_tags 的互斥校验在调用方分叉处统一完成。
	// 并发模型与替换路径一致：依赖 dscli 的项目级会话锁避免同进程并发写；
	// 外部进程修改文件时由 line_tag CAS 校验兜底。
	if insertLine < 1 {
		err = fmt.Errorf("insert_before_line must be >= 1, got %d", insertLine)
		return result, warning, err
	}
	if content == "" {
		err = fmt.Errorf("insert_before_line requires non-empty content (use the replace path with empty content to delete lines)")
		return result, warning, err
	}

	// 打开文件；不存在时仅允许 insert_before_line=1（等价于创建文件）
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			if insertLine != 1 {
				err = fmt.Errorf("cannot insert into non-existent file before line %d, only line 1 is valid", insertLine)
				return result, warning, err
			}
			writeContent := content
			if !strings.HasSuffix(writeContent, "\n") {
				writeContent += "\n"
			}
			if err = os.WriteFile(fullPath, []byte(writeContent), 0o644); err != nil {
				err = fmt.Errorf("failed to create file: %w", err)
				return result, warning, err
			}
			outfmt.Notice("创建文件 \"%s\" 并写入 %d 行内容（insert_before_line=1）", displayPath, strings.Count(content, "\n")+1)
			result = fmt.Sprintf("成功创建文件并写入 %d 行内容", strings.Count(content, "\n")+1)
			if showContext {
				if ctxStr := AppendWriteFileContext(displayPath); ctxStr != "" {
					result += ctxStr
				}
			}
			return result, warning, err
		}
		err = fmt.Errorf("failed to open file: %w", err)
		return result, warning, err
	}
	defer file.Close()

	// 读取所有行
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		err = fmt.Errorf("failed to read file: %w", err)
		return result, warning, err
	}

	// 插入点范围校验：合法插入点只有 1..len+1（len+1 = 追加末尾）
	if insertLine > len(lines)+1 {
		err = fmt.Errorf("insert_before_line=%d out of range: file has %d lines, max insert point is %d", insertLine, len(lines), len(lines)+1)
		return result, warning, err
	}

	// CAS：line_tag 校验插入点行；追加末尾时校验最后一行；空文件无行可校验
	lineTag := toolcall.ToolArgsValue(args, "line_tag", "")
	if lineTag != "" {
		if len(lineTag) != 4 {
			err = fmt.Errorf("line_tag must be exactly 4 characters, got %q (%d chars)", lineTag, len(lineTag))
			return result, warning, err
		}
		if len(lines) > 0 {
			verifyIdx := min(insertLine-1, len(lines)-1)
			if err = verifyLineTags(lines, verifyIdx, []string{lineTag}); err != nil {
				return result, warning, err
			}
			if stripped, didStrip := stripCASTags(content, []string{lineTag}); didStrip {
				content = stripped
				warning = "注意：已自动去除 content 中的 CAS tag 前缀（匹配已验证的 tag）。"
			}
		}
	}

	// 构建新内容：插入点之前的行 + 新内容 + 插入点及之后的行。
	// TrimSuffix 去掉末尾换行符，避免 Split 产生多余空行（与替换路径一致）。
	contentLineCount := strings.Count(content, "\n") + 1
	if strings.HasSuffix(content, "\n") {
		contentLineCount = strings.Count(content, "\n")
	}
	contentLines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	newLines := make([]string, 0, len(lines)+contentLineCount)
	newLines = append(newLines, lines[:insertLine-1]...)
	newLines = append(newLines, contentLines...)
	newLines = append(newLines, lines[insertLine-1:]...)

	// 写回文件，确保末尾有换行符
	var contentBuilder strings.Builder
	for i, line := range newLines {
		contentBuilder.WriteString(line)
		if i < len(newLines)-1 {
			contentBuilder.WriteString("\n")
		}
	}
	writeContent := contentBuilder.String()
	if writeContent != "" && !strings.HasSuffix(writeContent, "\n") {
		writeContent += "\n"
	}
	if err = os.WriteFile(fullPath, []byte(writeContent), 0o644); err != nil {
		err = fmt.Errorf("failed to write file: %w", err)
		return result, warning, err
	}

	// 结果描述：区分"插入到某行之前"与"追加到末尾"
	rangeDesc := fmt.Sprintf("第%d行之前", insertLine)
	if insertLine == len(lines)+1 {
		rangeDesc = fmt.Sprintf("末尾（第%d行之后）", len(lines))
	}
	outfmt.Notice("插入%d行内容到文件 \"%s\" %s", contentLineCount, displayPath, rangeDesc)
	result = fmt.Sprintf("成功插入 %d 行内容到文件 \"%s\" %s", contentLineCount, displayPath, rangeDesc)

	// 编辑后上下文窗口：插入点即新内容起点；oldReplaced=0（无覆盖）。
	// endLine=insertLine-1 使偏移警告从原第 insertLine 行（第一个受影响行）起算。
	if showContext {
		if ctxStr := AppendEditContext(displayPath, insertLine, insertLine-1, 0, contentLineCount); ctxStr != "" {
			result += ctxStr
		}
	}

	// Run flycheck on the written file and append issues as suggestion
	if flyResult, _, flyErr := flycheck.Flycheck(ctx, fullPath); flyErr == nil && flyResult != "" {
		if warning != "" {
			warning += "\n\n"
		}
		warning += flyResult
	}

	return result, warning, err
}
