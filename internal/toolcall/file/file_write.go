package file

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dscli_context "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/flycheck"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed file_write.md
var file_write_md string

const (
	// previewLastChars 截断时预览显示的最后字符数
	previewLastChars = 2048
	// maxOutputTokens LLM最大输出token限制（用于错误信息）
	maxOutputTokens = 327680 // 320K
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "write_file",
		Description: file_write_md,
		Strict:      true,
		// write_file_with_line_range 是旧名:2026-08 合并四个文件工具为两个 (read_file/write_file),
		// 注册为别名让历史会话中的旧调用名继续可用 (GetToolDef 解析,工具列表不显示)。
		Aliases: []string{"write_file_with_line_range"},
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, e.g. main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write; empty string to delete lines, max 524288 chars (line edit: 4096 chars recommended)",
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
		Handler:  handleWriteFile,
	})
}

// fileLineRangeKeys 是行编辑模式的"位置"参数：任一存在即走行编辑语义。
// 全部缺失则走全文件覆盖语义（旧 write_file 行为）。
// 注意 line_tag/line_tags 单独存在时不算位置参数——它们必须配合
// start_line/end_line/insert_before_line 才有意义（见 handleWriteFile）。
var fileLineRangeKeys = []string{"start_line", "end_line", "insert_before_line"}

// hasLineRangeParams 判断调用是否为行编辑模式（依据位置行参数）。
func hasLineRangeParams(args ToolArgs) bool {
	for _, k := range fileLineRangeKeys {
		if _, ok := args[k]; ok {
			return true
		}
	}
	return false
}

// handleWriteFile 写入文件。
//
// 2026-08 合并 write_file / write_file_with_line_range 两个工具：
//   - 提供任一位置行参数（start_line/end_line/insert_before_line）
//     → 行编辑语义（旧 write_file_with_line_range）；
//   - 否则 → 全文件覆盖语义（旧 write_file：覆盖整个文件、自动创建父目录）。
//
// 参数集与旧 write_file_with_line_range 完全一致（无 append 参数）：
// 追加内容请用 insert_before_line=N+1（N=文件总行数）或先 read_file 再行编辑。
// 历史调用中的 append 参数被明确拒绝（见下），防止静默截断文件。
func handleWriteFile(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleWriteFile")
	defer span.Finish()

	// 历史 append 参数保护：旧 write_file 的 append=true 合并时已移除，
	// 若静默落入全文件覆盖会用 O_TRUNC 截断文件——数据损坏。显式拒绝并
	// 指引等效操作（insert_before_line=total+1），绝不静默执行。
	if v, ok := args["append"]; ok {
		if b, isBool := v.(bool); isBool && b {
			return "", "", fmt.Errorf(
				"write_file no longer supports append=true (removed in the 2026-08 tool merge); " +
					"use insert_before_line=<total+1> to append, read_file first to get the total",
			)
		}
		// append=false 等价于全文件覆盖：丢弃即可。
		delete(args, "append")
	}

	// CAS tag 污染检查是两种模式共同的硬性前置（content 必须是纯文件内容），
	// 统一在分派前执行，保证两种模式行为一致且错误文案相同。
	if n := detectCASTags(toolcall.ToolArgsValue(args, "content", "")); n >= casTagThreshold {
		return "", "", fmt.Errorf(
			"内容包含疑似 read_file CAS tag（检测到 %d 行含有 CAS tag 前缀）。\n"+
				"write_file 接收的是文件内容，不应包含 read_file 输出的行首 4 字符 TAG（如 \"Q8fA\" 或 \"[Q8fA]\"）。\n"+
				"请去除这些 CAS tag 前缀后重试。",
			n,
		)
	}

	// line_tag/line_tags 必须配合位置行参数：单独使用时无法确定目标行，
	// 静默回退会错误地编辑第 1 行。显式报错让调用方补齐参数。
	for _, tagKey := range []string{"line_tag", "line_tags"} {
		if _, ok := args[tagKey]; ok && !hasLineRangeParams(args) {
			return "", "", fmt.Errorf(
				"%s requires start_line/end_line (or insert_before_line) to select the target range; "+
					"it cannot be used alone", tagKey,
			)
		}
	}

	if hasLineRangeParams(args) {
		return handleWriteFileWithLineRange(ctx, args)
	}
	return handleWriteFileFull(ctx, args)
}

// handleWriteFileFull 全文件覆盖语义（旧 write_file 行为）：
// 覆盖整个文件（或创建新文件），自动创建父目录，末尾确保换行符。
// 截断（finish_reason=length）时写入可用内容并提示模型用 insert_before_line 续写。
func handleWriteFileFull(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleWriteFileFull")
	defer span.Finish()
	truncated := dscli_context.ContextValue(ctx, dscli_context.FinishReasonLengthKey, false)
	path := toolcall.ToolArgsValue(args, "path", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	showContext := toolcall.ToolArgsValue(args, "context", true)
	lastlines := ""
	if truncated {
		runes := []rune(content)
		start := max(len(runes)-previewLastChars, 0)
		lastlines = string(runes[start:])
	}

	if path == "" {
		err = fmt.Errorf("文件路径 path 不能为空")
		if truncated {
			warning = fmt.Sprintf("内容截断，因为内容长度 %d 超过了最大输出 Tokens 要求 %d，请严格遵守 write_file 要求，严格控制输出。", len(content), maxOutputTokens)
		}
		return result, warning, err
	}

	fullPath := ResolvePath(ctx, path)
	dirPath := filepath.Dir(fullPath)
	var fi os.FileInfo
	fi, err = os.Stat(dirPath)
	if err == nil && !fi.IsDir() {
		err = fmt.Errorf("%s is not directory", dirPath)
		return result, warning, err
	}

	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, 0o755)
	}

	if err != nil {
		err = fmt.Errorf("failed to get or create directory %s: %w", dirPath, err)
		return result, warning, err
	}

	var file *os.File
	file, err = os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		err = fmt.Errorf("无法打开文件: %w", err)
		return result, warning, err
	}
	defer file.Close()

	// 写入内容
	if _, err = file.WriteString(content); err != nil {
		err = fmt.Errorf("写入内容失败: %w", err)
		return result, warning, err
	}

	// POSIX 约定：文本文件应以换行符结尾
	// 确保写入的内容末尾有换行符
	if content != "" && !strings.HasSuffix(content, "\n") {
		if _, err = file.WriteString("\n"); err != nil {
			err = fmt.Errorf("写入尾随换行符失败: %w", err)
			return result, warning, err
		}
	}

	lines := countContentLines(content)
	outfmt.Notice("写入文件 \"%s\"，%d 行", path, lines)
	result = fmt.Sprintf("成功写入文件 \"%s\"，%d 行。", path, lines)
	if truncated {
		// 截断续写引导：文件当前有 lines 行，模型应按顺序追加剩余内容。
		// insert_before_line=lines+1 等价于追加到文件末尾。
		warning = fmt.Sprintf(`此次写入文件 %s 的内容是截断的内容（共 %d 行）。
请从上次输出内容的最后一完整行继续生成，并调用工具 write_file(path="%s", insert_before_line=%d, content="...继续生成的内容...")
继续追加，为帮助你找到继续生成的点，现把上次截断内容最后几行展示给你：
---
%s---
如果觉得信息不足以继续生成，可以停下来询问。`, path, lines, path, lines+1, lastlines)
	}

	// 编辑后上下文窗口
	if showContext && !truncated {
		ctxStr := AppendWriteFileContext(path)
		if ctxStr != "" {
			result += ctxStr
		}
	}

	// Run flycheck on the written file and append issues to suggestion
	warning = appendFlycheckWarning(ctx, warning, path)

	return result, warning, err
}

// handleWriteFileWithLineRange 行编辑语义（旧 write_file_with_line_range 行为）：
// 替换/删除/插入指定行范围，支持 CAS tag 校验防竞态写入。
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
	contentLineCount := countContentLines(content)

	// 读取原文件所有行
	file, err := os.Open(fullPath)
	if err != nil {
		// 如果文件不存在，创建一个空文件
		if os.IsNotExist(err) {
			result, handled, err := writeMissingFile(fullPath, displayPath, startLine, content, showContext, contentLineCount)
			if handled {
				return result, warning, err
			}
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

	content, casVerified, warning, err := resolveCASTags(args, lines, startLine, content)
	if err != nil {
		return result, warning, err
	}

	// --- 构建新内容 ---
	newLines := buildReplacementLines(lines, startLine, endLine, content)

	// 将新内容写回文件，确保末尾有换行符
	writeContent := joinLinesWithNewline(newLines)
	err = os.WriteFile(fullPath, []byte(writeContent), 0o644)
	if err != nil {
		err = fmt.Errorf("failed to write file: %w", err)
		return result, warning, err
	}

	// 记录操作日志
	operation, rangeDesc, oldReplaced, linesChanged := describeReplacement(content, startLine, endLine, oldTotalLines, contentLineCount)

	warning = appendLengthMismatchWarning(warning, content, endLine, casVerified, oldReplaced, contentLineCount)

	outfmt.Notice("%s文件 \"%s\" 行范围 %s，影响 %d 行", operation, displayPath, rangeDesc, linesChanged)

	// 构建最终结果
	result = fmt.Sprintf("成功%s文件 \"%s\" 行范围 %s", operation, displayPath, rangeDesc)

	// 编辑后上下文窗口
	result = appendEditContext(result, showContext, displayPath, startLine, endLine, oldTotalLines, oldReplaced, contentLineCount)

	// Run flycheck on the written file and append issues as suggestion
	warning = appendFlycheckWarning(ctx, warning, fullPath)

	return result, warning, err
}

// countContentLines 返回 content 的"行数"语义：
// 空串为 0 行；以 \n 结尾时等于换行符数（尾随换行不产生空行）；否则 \n 数 + 1。
func countContentLines(content string) int {
	if content == "" || strings.HasSuffix(content, "\n") {
		return strings.Count(content, "\n")
	}
	return strings.Count(content, "\n") + 1
}

// joinLinesWithNewline 以 \n 连接行切片；非空结果保证以 \n 结尾（POSIX 文本约定）。
func joinLinesWithNewline(lines []string) string {
	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(line)
		if i < len(lines)-1 {
			sb.WriteString("\n")
		}
	}
	out := sb.String()
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

// appendFlycheckWarning 在已写文件上运行 flycheck，把问题追加到 warning（如有）。
func appendFlycheckWarning(ctx context.Context, warning, path string) string {
	if flyResult, _, flyErr := flycheck.Flycheck(ctx, path); flyErr == nil && flyResult != "" {
		if warning != "" {
			warning += "\n\n"
		}
		warning += flyResult
	}
	return warning
}

// writeMissingFile 处理替换路径中"目标文件不存在"的分支:
// 只允许从第 1 行开始写入；空内容创建空文件；否则写入内容并按需追加上下文窗口。
// handled=true 表示该分支已完整处理本次操作，调用方直接返回。
func writeMissingFile(fullPath, displayPath string, startLine int, content string, showContext bool, contentLineCount int) (result string, handled bool, err error) {
	// 对于新文件，只能从第1行开始写入
	if startLine != 1 {
		err = fmt.Errorf("cannot write to non-existent file at line %d, must start from line 1", startLine)
		return result, true, err
	}

	// 创建新文件并写入内容
	if content == "" {
		// 空内容，创建空文件
		var newFile *os.File
		newFile, err = os.Create(fullPath)
		if err != nil {
			err = fmt.Errorf("failed to create file: %w", err)
			return result, true, err
		}
		newFile.Close()
		outfmt.Notice("创建空文件 \"%s\"", displayPath)
		result = "成功创建空文件"
		return result, true, err
	}

	// 写入内容到新文件，确保末尾换行
	writeContent := content
	if writeContent != "" && !strings.HasSuffix(writeContent, "\n") {
		writeContent += "\n"
	}
	err = os.WriteFile(fullPath, []byte(writeContent), 0o644)
	if err != nil {
		err = fmt.Errorf("failed to write to new file: %w", err)
		return result, true, err
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
	return result, true, err
}

// resolveCASTags 处理 line_tag/line_tags 参数: 互斥检查、长度检查、标签解析、
// 与文件内容校验（verifyLineTags），并剥离 content 中匹配已验证 tag 的前缀。
// casVerified=true 表示调用者执行了 CAS 校验（后续的长度突变告警因此跳过）。
func resolveCASTags(args ToolArgs, lines []string, startLine int, content string) (contentOut string, casVerified bool, warning string, err error) {
	// --- CAS tag verification (antirez-style check-and-set) ---
	// 如果提供了 line_tag 或 line_tags，写入前校验标签匹配
	lineTag := toolcall.ToolArgsValue(args, "line_tag", "")
	lineTags := toolcall.ToolArgsValue(args, "line_tags", "")
	var expectedTags []string

	if lineTag != "" || lineTags != "" {
		if lineTag != "" && lineTags != "" {
			err = fmt.Errorf("cannot specify both line_tag and line_tags; use line_tag for single-line edits, line_tags for multi-line")
			return content, true, warning, err
		}
		if lineTag != "" {
			if len(lineTag) != 4 {
				err = fmt.Errorf("line_tag must be exactly 4 characters, got %q (%d chars)", lineTag, len(lineTag))
				return content, true, warning, err
			}
			expectedTags = []string{lineTag}
		} else {
			expectedTags, err = parseLineTags(lineTags)
			if err != nil {
				err = fmt.Errorf("failed to parse line_tags: %w", err)
				return content, true, warning, err
			}
		}

		// Verify tags against actual file content at startLine
		if err = verifyLineTags(lines, startLine-1, expectedTags); err != nil {
			return content, true, warning, err
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
	return content, len(expectedTags) > 0, warning, nil
}

// buildReplacementLines 按 startLine/endLine 语义拼接新文件行:
// start 之前、空行填充、新内容（去掉尾随 \n 避免 Split 空行）、end 之后。
func buildReplacementLines(lines []string, startLine, endLine int, content string) []string {
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
	return newLines
}

// describeReplacement 计算操作描述（替换/删除）、行范围描述、被替换原始行数与净行数。
func describeReplacement(content string, startLine, endLine int, oldTotalLines, contentLineCount int) (operation, rangeDesc string, oldReplaced, linesChanged int) {
	operation = "替换"
	if content == "" {
		operation = "删除"
	}

	rangeDesc = fmt.Sprintf("第%d行 - 第%d行", startLine, endLine)
	if endLine == -1 {
		rangeDesc = fmt.Sprintf("第%d行 - 末尾", startLine)
	}

	// 计算被替换的原始行数
	oldReplaced = 0
	if endLine == -1 {
		oldReplaced = max(0, oldTotalLines-startLine+1)
	} else {
		oldReplaced = max(0, min(endLine, oldTotalLines)-startLine+1)
	}
	linesChanged = oldReplaced
	if content != "" {
		linesChanged = contentLineCount
	}
	return operation, rangeDesc, oldReplaced, linesChanged
}

// appendLengthMismatchWarning 在无 CAS 校验、非空内容、显式 end_line 时检测
// "行号错位覆盖"信号并追加告警文案。
func appendLengthMismatchWarning(warning string, content string, endLine int, casVerified bool, oldReplaced, contentLineCount int) string {
	// 长度突变告警（无 CAS tag 时）：替换区域行数与写入内容行数严重不匹配
	// 是"行号错位覆盖"的典型信号——AI 本意在某处插入新内容，但目标区域实际
	// 行数远少于内容行数，静默覆盖了不该覆盖的内容。仅告警不阻断：合法的大
	// 规模替换（如展开函数体）也会触发，由 LLM 自行判断。
	if content != "" && endLine != -1 && !casVerified {
		if m := warnOnLengthMismatch(oldReplaced, contentLineCount); m != "" {
			if warning != "" {
				warning += "\n\n"
			}
			warning += m
		}
	}
	return warning
}

// appendEditContext 计算有效结束行并把编辑后上下文窗口追加到 result。
func appendEditContext(result string, showContext bool, displayPath string, startLine, endLine, oldTotalLines, oldReplaced, contentLineCount int) string {
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
	return result
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
	contentLineCount := countContentLines(content)
	contentLines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	newLines := make([]string, 0, len(lines)+contentLineCount)
	newLines = append(newLines, lines[:insertLine-1]...)
	newLines = append(newLines, contentLines...)
	newLines = append(newLines, lines[insertLine-1:]...)

	// 写回文件，确保末尾有换行符
	writeContent := joinLinesWithNewline(newLines)
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
	warning = appendFlycheckWarning(ctx, warning, fullPath)

	return result, warning, err
}
