package ask

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/shell"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed code_review.md
var code_review_md string

var codeReviewTool = toolcall.ToolDef{
	Name:        "code_review",
	DisplayName: "Code Review",
	Description: code_review_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Required, background and focus of this commit, 1-1024 chars",
			},
			"test_command": map[string]any{
				"type":        "string",
				"description": "Optional test command, default empty skips tests, 1-128 chars",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Number of commits to review, e.g. '-1' (last), '-2' (last 2), default '-1'",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 900). The expert may run multiple tool-call rounds; set longer for very large projects with many tests.",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "check",
	// The expert may run multiple tool-call rounds (each needs a browser
	// session + a model reply), so the budget must cover a full loop, not
	// just a single WebChat exchange.
	Timeout: 15 * time.Minute,
	Handler: handleCodeReview,
}

func init() {
	// WebChat is always available (free DeepSeek V4 Pro) — no API key needed.
	toolcall.RegisterTool(codeReviewTool)
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleCodeReview")
	defer span.Finish()
	summary := toolcall.ToolArgsValue(args, "summary", "")
	testCommand := toolcall.ToolArgsValue(args, "test_command", "")
	since := toolcall.ToolArgsValue(args, "since", "-1")

	if summary == "" {
		outfmt.Println("❌ 必须提供提交摘要")
		err = fmt.Errorf("必须提供提交摘要")
		return result, warning, err
	}

	// 校验 since 格式并提取 N
	n, err := parseSince(since)
	if err != nil {
		outfmt.Printf("❌ since 参数格式错误: %v\n", err)
		return result, warning, err
	}

	// 检查是否有未提交的更改（staged + unstaged，忽略 untracked）
	fmt.Println("🔍 检查是否有未提交的更改...")
	statusScript := `git status --porcelain | grep -v '^??'`
	status, shellErr := shell.SimpleExecute(ctx, statusScript)
	if shellErr != nil {
		// grep 返回非零退出码表示没有匹配，这是正常情况
		status = ""
	}

	if status != "" {
		outfmt.Println("❌ 检测到未提交的更改")
		outfmt.Println("当前状态：")
		outfmt.Println(status)
		err = fmt.Errorf("请使用 'git status' 查看详情，并使用 'git add' 和 'git commit' 提交所有更改后再进行审查")
		return result, warning, err
	}

	outfmt.Println("✅ 没有未提交的更改")

	if testCommand != "" {
		outfmt.Println("🔍 运行单元测试:", testCommand)
		testOutput := ""
		testOutput, err = shell.SimpleExecute(ctx, testCommand)
		if err != nil {
			outfmt.Println("❌ 单元测试未通过")
			errorMsg := fmt.Sprintf("单元测试未通过，请修复测试后再审查。\n测试命令：%s\n", testCommand)
			if testOutput != "" {
				// 截断过长的输出
				outputLines := strings.Split(testOutput, "\n")
				if len(outputLines) > 20 {
					errorMsg += "测试输出（前20行）：\n" + strings.Join(outputLines[:20], "\n")
					errorMsg += fmt.Sprintf("\n... 还有%d行输出", len(outputLines)-20)
				} else {
					errorMsg += "测试输出：\n" + testOutput
				}
			}
			outfmt.Println("❌ 单元测试失败")
			err = fmt.Errorf("%s: %w", errorMsg, err)
			return result, warning, err
		}
		if testOutput != "" {
			outfmt.Println(testOutput)
		}
		outfmt.Println("✅ 单元测试通过")
	}

	// 获取最新的提交信息
	logScript := fmt.Sprintf(`git log --oneline %s`, since)
	log, err := shell.SimpleExecute(ctx, logScript)
	if err != nil {
		outfmt.Println("❌ 获取提交历史失败")
		err = fmt.Errorf("获取提交历史失败: %w", err)
		return result, warning, err
	}

	if strings.TrimSpace(log) == "" {
		outfmt.Println("❌ 没有找到提交记录")
		err = fmt.Errorf("没有找到提交记录，请先提交代码")
		return result, warning, err
	}

	outfmt.Println("📝 提交信息:")
	outfmt.Println(log)

	// 获取完整的提交信息用于构建请求
	fullLogScript := fmt.Sprintf(`git log --format="%%B" %s`, since)
	fullLog, err := shell.SimpleExecute(ctx, fullLogScript)
	if err != nil {
		fullLog = log // 如果失败，使用简短的log
	}

	// 生成patch
	patchScript := fmt.Sprintf(`git --no-pager format-patch --stdout %s`, since)
	patch, err := shell.SimpleExecute(ctx, patchScript)
	if err != nil {
		fmt.Println("❌ 生成patch失败")
		err = fmt.Errorf("生成patch失败: %w", err)
		return result, warning, err
	}

	// 获取修改文件的全文 + 仓库指南（AGENTS.md 提供设计决策上下文）
	files := collectFileContents(ctx, n)
	guide := readProjectGuide()

	// 构建审查请求并检查输入长度（超限时分层截断，盲区写入 warning）
	structuredRequest, warning := truncateReviewRequest(summary, fullLog, patch, files, guide)
	outfmt.Printf("📤 发送代码审查请求到 DeepSeek Web（免费 V4 Pro）...\n%s\n", structuredRequest)
	result, err = AskExpertWithRole(ctx, structuredRequest, "review")
	if err != nil {
		err = fmt.Errorf("代码审查失败: %w", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 代码审查结果\n%s\n", result)
	return result, warning, err
}

// parseSince 解析 since 参数，返回提交数 N。
// since 必须为 "-N" 格式（如 "-1", "-2", "-3"）。
func parseSince(since string) (int, error) {
	if !strings.HasPrefix(since, "-") {
		return 0, fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	n, err := strconv.Atoi(since[1:])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	return n, nil
}

// ---------- 仓库指南 ----------

// maxGuideLen caps the AGENTS.md excerpt included in the review request.
const maxGuideLen = 6000

// readProjectGuide reads the repository's AGENTS.md (if any) so the expert sees
// the project's design decisions and conventions. Oversized guides are capped.
func readProjectGuide() string {
	return readProjectGuideFrom("AGENTS.md")
}

// readProjectGuideFrom is the testable core of readProjectGuide.
func readProjectGuideFrom(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	guide := string(b)
	if len(guide) > maxGuideLen {
		// 截断到 rune 边界，避免切破多字节字符产生非法 UTF-8
		cut := maxGuideLen
		for cut > 0 && !utf8.RuneStart(guide[cut]) {
			cut--
		}
		guide = guide[:cut] + "\n..[AGENTS.md 截断，仅保留前 " + strconv.Itoa(cut) + " 字符]..\n"
	}
	return guide
}

// ---------- 文件内容 ----------

const (
	// maxFullFileSize: 超过此大小的文件不插入全文，diff 仍可审查（大文件如
	// 生成的代码、巨型单文件，全文会挤占审查预算且收益低）。
	maxFullFileSize = 100 * 1024
	// maxStoredFileSize: 超过此大小连上下文摘录也不读取（摘录仅需改动点附近）。
	maxStoredFileSize = 4 << 20
)

// fileContent 描述一个被修改的文件。
type fileContent struct {
	name    string // 文件路径（新侧）
	content string // HEAD 中的全文；不可用时为空
	note    string // 说明（已删除/二进制/过大/无法读取）
}

// collectFileContents 收集最近 n 个提交中修改文件的 HEAD 内容。
// 返回有序列表（与 git diff --name-status 输出一致）。
func collectFileContents(ctx context.Context, n int) []fileContent {
	filesScript := fmt.Sprintf(`git diff --name-status HEAD~%d HEAD`, n)
	output, err := shell.SimpleExecute(ctx, filesScript)
	if err != nil || strings.TrimSpace(output) == "" {
		return nil
	}

	var files []fileContent
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		// 格式: STATUS[ old]\tfile  (R/C 状态有 3 字段: R100\told\tnew)
		// 始终取最后一个字段作为文件路径
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0]
		file := fields[len(fields)-1]

		f := fileContent{name: file}
		if status == "D" {
			f.note = "[文件已删除]"
			files = append(files, f)
			continue
		}

		// 先查大小，避免为大文件读取全文
		sizeScript := "git cat-file -s HEAD:" + shellQuote(file)
		sizeStr, err := shell.SimpleExecute(ctx, sizeScript)
		if err != nil {
			f.note = fmt.Sprintf("[无法读取文件: %v]", err)
			files = append(files, f)
			continue
		}
		size, _ := strconv.Atoi(strings.TrimSpace(sizeStr))
		if size > maxStoredFileSize {
			f.note = fmt.Sprintf("[文件过大 (%d bytes)，未读取全文，diff 可审查]", size)
			files = append(files, f)
			continue
		}

		content, err := shell.SimpleExecute(ctx, "git show HEAD:"+shellQuote(file))
		if err != nil {
			f.note = fmt.Sprintf("[无法读取文件: %v]", err)
		} else if content == "" {
			f.note = "[空文件]"
		} else if strings.IndexByte(content, 0) >= 0 {
			f.note = "[二进制文件，不插入全文]"
		} else {
			f.content = content
		}
		files = append(files, f)
	}
	return files
}

// shellQuote 用单引号包裹 s，保证含空格/特殊字符的路径可安全嵌入 shell 命令。
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// formatFullContents 渲染文件全文区段。超过 maxFullFileSize 的文件只留说明，
// 不插入全文，diff 已足够审查其改动。
func formatFullContents(files []fileContent) string {
	var sb strings.Builder
	for _, f := range files {
		switch {
		case f.content == "":
			fmt.Fprintf(&sb, "## File: %s %s\n", f.name, f.note)
		case len(f.content) > maxFullFileSize:
			fmt.Fprintf(&sb, "## File: %s [文件过大 (%d bytes)，不插入全文，diff 可审查]\n", f.name, len(f.content))
		default:
			fmt.Fprintf(&sb, "## File: %s\n```%s\n%s\n```\n", f.name, excerptLang(f.name), f.content)
		}
	}
	return sb.String()
}

// ---------- 请求构建 ----------

// buildCodeReviewRequest 组装审查请求。patch/fileSection 为空时省略对应区段。
func buildCodeReviewRequest(summary, commitLog, patch, guide, fileSection string) string {
	req := "## Commit Background\n" + summary + "\n\n## Commit Message\n" + commitLog
	if guide != "" {
		req += "\n\n## Project Guide (AGENTS.md)\n" + guide
	}
	if patch != "" {
		req += "\n\n## Code Changes\n" + patch
	}
	if fileSection != "" {
		req += "\n\n## File Contents\n" + fileSection
	}
	return req
}

// ---------- diff hunk 解析 ----------

// hunkRange 表示一个 hunk 的新侧行范围（1-based 闭区间）。
// 纯删除的 hunk 为 end < start 的空范围，锚点 start 是删除位置。
type hunkRange struct{ start, end int }

// hunkRe 匹配 "@@ -a,b +c,d @@"（b/d 可省略，默认为 1）。
var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parsePatchHunks 从 unified diff 中提取每个文件的新侧 hunk 范围。
// 兼容 format-patch（含邮件头）与 git diff 输出；支持新增/删除/重命名文件。
// 注意：路径含特殊字符时 git 会加引号（+++ "b/a b.go"），此类文件解析不到
// hunk，但 diff 区段仍在，摘录会优雅跳过。
func parsePatchHunks(patch string) map[string][]hunkRange {
	hunks := make(map[string][]hunkRange)
	cur := ""
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			cur = "" // 新文件区段，等待 +++ b/ 行
		case strings.HasPrefix(line, "+++ b/"):
			cur = strings.TrimPrefix(line, "+++ b/")
			if cur == "/dev/null" {
				cur = "" // 已删除文件：无新侧内容
			}
		case cur != "" && strings.HasPrefix(line, "@@ "):
			if h, ok := parseHunk(line); ok {
				hunks[cur] = append(hunks[cur], h)
			}
		}
	}
	return hunks
}

// parseHunk 解析单个 "@@ ... @@" 行，返回新侧行范围。
func parseHunk(line string) (hunkRange, bool) {
	m := hunkRe.FindStringSubmatch(line)
	if m == nil {
		return hunkRange{}, false
	}
	start, _ := strconv.Atoi(m[3])
	count := 1
	if m[4] != "" {
		count, _ = strconv.Atoi(m[4])
	}
	return hunkRange{start: start, end: start + count - 1}, true
}

// splitPatchByFile 将 patch 按文件拆分为独立区段（用于逐文件丢弃）。
func splitPatchByFile(patch string) []namedSection {
	var secs []namedSection
	var cur *namedSection
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			if cur != nil {
				secs = append(secs, *cur)
			}
			cur = &namedSection{text: line + "\n"}
			continue
		}
		if cur != nil {
			cur.text += line + "\n"
			if strings.HasPrefix(line, "+++ b/") {
				cur.name = strings.TrimPrefix(line, "+++ b/")
			}
		}
	}
	if cur != nil {
		secs = append(secs, *cur)
	}
	return secs
}

// ---------- 上下文摘录 ----------

const (
	// defLookback: 向上扫描定义起点的最大行数。
	defLookback = 100
	// maxDefLen: 展开后的定义窗口最大行数（防止巨型函数挤爆预算）。
	maxDefLen = 200
)

// win 是 0-based 半开行区间。
type win struct{ start, end int }

// namedSection 是带文件名的文本区段（用于逐文件保留/丢弃）。
type namedSection struct {
	name string
	text string
}

// excerptLang 根据文件扩展名推断代码块语言标签与定义识别规则。
func excerptLang(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go":
		return "go"
	case ".el", ".elisp":
		return "elisp"
	case ".py":
		return "python"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "js"
	case ".ts", ".tsx", ".mts", ".cts":
		return "ts"
	case ".c", ".h":
		return "c"
	case ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx":
		return "cpp"
	case ".java":
		return "java"
	case ".rs":
		return "rs"
	case ".sh", ".bash":
		return "sh"
	default:
		return ""
	}
}

// defStartRe 识别各语言的定义起始行（启发式，未命中时回退到原始窗口）。
var defStartRe = map[string]*regexp.Regexp{
	"go":     regexp.MustCompile(`^\s*(func|type|var|const)\s`),
	"elisp":  regexp.MustCompile(`^\s*\((def\w+|cl-defun|cl-defmacro|cl-defstruct|cl-defgeneric|cl-defmethod|cl-defvar|cl-defconst)\b`),
	"python": regexp.MustCompile(`^\s*(async\s+def|def|class)\s`),
	"js":     regexp.MustCompile(`^\s*(export\s+)?(default\s+)?(async\s+)?function\b|^\s*(export\s+)?(default\s+)?class\b`),
	"ts":     regexp.MustCompile(`^\s*(export\s+)?(default\s+)?(async\s+)?function\b|^\s*(export\s+)?(default\s+)?(abstract\s+)?class\b`),
	"c":      regexp.MustCompile(`^\s*(static\s+|inline\s+|const\s+|extern\s+|unsigned\s+|signed\s+)?[\w\s\*&]+\([^;{}]*\)\s*\{`),
	"cpp":    regexp.MustCompile(`^\s*(static\s+|inline\s+|const\s+|constexpr\s+|extern\s+)?[\w\s\*&:<>]+\([^;{}]*\)\s*\{|^\s*(struct|class|enum|union)\s+\w+`),
	"java":   regexp.MustCompile(`^\s*(?:(?:public|protected|private|static|final|synchronized|abstract|native|default)\s+)?[\w<>,.?\s\[\]]+\([^;{}]*\)\s*\{|^\s*(?:(?:public|protected|private|static|final)\s+)?(?:class|interface|enum|record)\s+\w+`),
	"rs":     regexp.MustCompile(`^\s*(pub(?:\([^)]*\))?\s+)?(fn|struct|enum|impl|trait|mod|type|const|static|macro_rules!)\b`),
	"sh":     regexp.MustCompile(`^\s*(function\s+)?[a-zA-Z_][a-zA-Z0-9_]*\s*\(\s*\)\s*\{`),
}

// defSkipRe 排除被 defStartRe 误判为定义的控制流关键字（如 "if (x) {"）。
var defSkipRe = map[string]*regexp.Regexp{
	"c":    regexp.MustCompile(`^\s*(if|for|while|switch|return|catch|do|else|case|sizeof)\b`),
	"cpp":  regexp.MustCompile(`^\s*(if|for|while|switch|return|catch|do|else|case|sizeof)\b`),
	"java": regexp.MustCompile(`^\s*(if|for|while|switch|return|catch|do|else|case)\b`),
}

// buildExcerptSection 为每个文件构建改动点上下文摘录区段。
// 每个 hunk 取 context 行上下文窗口；若窗口位于某定义块内（按语言启发式
// 识别），则展开为完整定义，让专家看到改动所在函数/宏的完整形态。
func buildExcerptSection(files []fileContent, hunks map[string][]hunkRange, context int) string {
	secs := perFileExcerptSections(files, hunks, context)
	var sb strings.Builder
	for _, s := range secs {
		sb.WriteString(s.text)
	}
	return sb.String()
}

// perFileExcerptSections 按文件返回摘录区段（用于逐文件保留/丢弃）。
func perFileExcerptSections(files []fileContent, hunks map[string][]hunkRange, context int) []namedSection {
	var secs []namedSection
	for _, f := range files {
		hs := hunks[f.name]
		if f.content == "" || len(hs) == 0 {
			// 无内容或无法定位 hunk 的文件：仅保留说明（极小，常被保留）
			if f.note != "" {
				secs = append(secs, namedSection{name: f.name, text: fmt.Sprintf("## File: %s %s\n", f.name, f.note)})
			}
			continue
		}

		lines := strings.Split(f.content, "\n")
		lang := excerptLang(f.name)
		var wins []win
		for _, h := range hs {
			s, e := hunkWindow(h, context, len(lines))
			if ds, de := findEnclosingDef(lines, h, context, lang); ds >= 0 {
				s, e = ds, de
			}
			wins = append(wins, win{s, e})
		}
		wins = mergeWindows(wins)

		var sb strings.Builder
		for _, w := range wins {
			fmt.Fprintf(&sb, "## File: %s (lines %d-%d of %d, context=%d)\n```%s\n%s\n```\n",
				f.name, w.start+1, w.end, len(lines), context, lang, strings.Join(lines[w.start:w.end], "\n"))
		}
		if sb.Len() > 0 {
			secs = append(secs, namedSection{name: f.name, text: sb.String()})
		}
	}
	return secs
}

// hunkWindow 返回 hunk 的原始上下文窗口（0-based 半开）。
// 纯删除 hunk（end < start）以 start 为锚点取两侧窗口。
func hunkWindow(h hunkRange, context, total int) (int, int) {
	s := h.start - 1 - context
	if s < 0 {
		s = 0
	}
	e := h.end + context
	if e > total {
		e = total
	}
	return s, e
}

// findEnclosingDef 向上查找包含 hunk 的定义块，返回 0-based 半开区间；
// 未找到或定义超长时返回 (-1, -1) 表示保持原始窗口。
func findEnclosingDef(lines []string, h hunkRange, context int, lang string) (int, int) {
	defRe, ok := defStartRe[lang]
	if !ok {
		return -1, -1
	}
	s, _ := hunkWindow(h, context, len(lines))
	lo := s - defLookback
	if lo < 0 {
		lo = 0
	}
	for i := s; i >= lo; i-- {
		if !defRe.MatchString(lines[i]) {
			continue
		}
		if skip, ok := defSkipRe[lang]; ok && skip.MatchString(lines[i]) {
			continue
		}
		end, ok := defEnd(lines, i, lang)
		if !ok {
			return -1, -1 // 无法配平（巨型块/字符串干扰）：回退原始窗口
		}
		// 定义必须与 hunk 有重叠（hunk 在定义之前结束则继续向上找外层）
		if end < h.start || i > h.end-1 {
			continue
		}
		if end-i > maxDefLen {
			if i+maxDefLen < h.end {
				return -1, -1 // hunk 超出展开上限：保持原始窗口
			}
			end = i + maxDefLen
		}
		return i, end
	}
	return -1, -1
}

// defEnd 返回定义块 start 行（含）的配平结束行（0-based 排他）。
func defEnd(lines []string, start int, lang string) (int, bool) {
	switch lang {
	case "elisp":
		return parenEnd(lines, start)
	case "python":
		return pythonDefEnd(lines, start)
	default:
		return braceEnd(lines, start)
	}
}

// maxDefScan is the line budget for balancing a definition block: double the
// expansion cap, so a long definition is detected (and then capped by
// findEnclosingDef) instead of silently falling back to the raw window.
const maxDefScan = 2 * maxDefLen

// braceEnd 用花括号配平（跳过字符串字面量）定位 Go/JS/Rust/C 等块结束。
// inStr 跨行保持：Go raw string 与 JS 模板字符串可跨行，字符串内的花括号
// 不得计入配平。
func braceEnd(lines []string, start int) (int, bool) {
	depth := 0
	inStr := byte(0) // 0 / '"' / '`'
	for i := start; i < len(lines) && i-start < maxDefScan; i++ {
		line := lines[i]
		for j := 0; j < len(line); j++ {
			c := line[j]
			if inStr != 0 {
				if c == '\\' {
					j++
				} else if c == inStr {
					inStr = 0
				}
				continue
			}
			switch c {
			case '"', '`':
				inStr = c
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					return i + 1, true
				}
			}
		}
	}
	return 0, false
}

// parenEnd 用括号配平（跳过字符串与注释）定位 elisp 的 defun/defmacro 结束。
// inStr 跨行保持：elisp 字符串可含换行。
func parenEnd(lines []string, start int) (int, bool) {
	depth := 0
	inStr := false
	for i := start; i < len(lines) && i-start < maxDefScan; i++ {
		line := lines[i]
		for j := 0; j < len(line); j++ {
			c := line[j]
			if inStr {
				if c == '\\' {
					j++
				} else if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case ';':
				j = len(line) // 注释到行尾
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					return i + 1, true
				}
			}
		}
	}
	return 0, false
}

// pythonDefEnd 以缩进定位 def/class 结束：下一个顶格非空行。
func pythonDefEnd(lines []string, start int) (int, bool) {
	for i := start + 1; i < len(lines) && i-start < maxDefScan; i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "@") {
			continue // 顶格注释/装饰器不属于定义体
		}
		return i, true
	}
	return 0, false
}

// mergeWindows 合并重叠或相邻的窗口。
func mergeWindows(wins []win) []win {
	if len(wins) == 0 {
		return nil
	}
	sort.Slice(wins, func(i, j int) bool { return wins[i].start < wins[j].start })
	merged := []win{wins[0]}
	for _, w := range wins[1:] {
		last := &merged[len(merged)-1]
		if w.start <= last.end+1 { // 重叠或相邻
			if w.end > last.end {
				last.end = w.end
			}
		} else {
			merged = append(merged, w)
		}
	}
	return merged
}

// ---------- 截断管线 ----------

// maxUserInputLen is the maximum length for the user portion of a code review
// request. DeepSeek's chat textarea enforces a total limit (~30k chars including
// the system prompt of ~2-3k), so we keep the user content under this threshold.
// 26000 keeps ~2.5k chars of margin under the combined limit.
const maxUserInputLen = 26000

// truncateReviewRequest 检查请求总长并在超限时分层截断。优先级：
//
//  0. 全文（小文件全部插入，最优）
//  1. 改动点上下文摘录（含完整函数定义），context 40→25→15→8→4 逐级收缩
//  2. 保留完整 diff，逐文件丢弃摘录（小文件优先保留，保证覆盖面）
//  3. 逐文件丢弃 diff 区段，剩余预算尽量保留摘录
//  4. 兜底：diff 保留末尾截断
//
// 返回截断后的请求与 warning；warning 明确列出被丢弃的文件（覆盖盲区）。
func truncateReviewRequest(summary, commitLog, patch string, files []fileContent, guide string) (string, string) {
	full := buildCodeReviewRequest(summary, commitLog, patch, guide, formatFullContents(files))
	if len(full) <= maxUserInputLen {
		return full, ""
	}
	origLen := len(full)
	var warns []string
	hunks := parsePatchHunks(patch)

	// Stage 1: 全文 → 摘录，context 逐级收缩
	for _, ctx := range []int{40, 25, 15, 8, 4} {
		sec := buildExcerptSection(files, hunks, ctx)
		req := buildCodeReviewRequest(summary, commitLog, patch, guide, sec)
		if len(req) <= maxUserInputLen {
			warns = append(warns, fmt.Sprintf("文件全文过大，已改用改动点上下文摘录（context=%d 行，含完整函数定义）", ctx))
			return req, truncateWarning(origLen, req, warns)
		}
	}

	// Stage 2: 保留完整 diff，逐文件丢弃摘录（最小优先）
	excerptSecs := perFileExcerptSections(files, hunks, 4)
	base := buildCodeReviewRequest(summary, commitLog, patch, guide, "")
	keptExc, droppedExc := dropUntilFits(base, excerptSecs, maxUserInputLen)
	for _, d := range droppedExc {
		warns = append(warns, fmt.Sprintf("已丢弃 %s 的上下文摘录（%d 字符），diff 仍可审查", d.name, len(d.text)))
	}
	req := buildCodeReviewRequest(summary, commitLog, patch, guide, joinNamed(keptExc))
	if len(req) <= maxUserInputLen {
		return req, truncateWarning(origLen, req, warns)
	}

	// Stage 3: diff 也超限，逐文件丢弃 diff 区段（最小优先），预算内尽量保留摘录
	diffSecs := splitPatchByFile(patch)
	base3 := buildCodeReviewRequest(summary, commitLog, "", guide, "")
	keptDiff, droppedDiff := dropUntilFits(base3, diffSecs, maxUserInputLen)
	for _, d := range droppedDiff {
		warns = append(warns, fmt.Sprintf("已丢弃 %s 的 diff（%d 字符）", d.name, len(d.text)))
	}
	base4 := buildCodeReviewRequest(summary, commitLog, joinNamed(keptDiff), guide, "")
	keptExc2, droppedExc2 := dropUntilFits(base4, excerptSecs, maxUserInputLen)
	for _, d := range droppedExc2 {
		warns = append(warns, fmt.Sprintf("已丢弃 %s 的上下文摘录（%d 字符），diff 仍可审查", d.name, len(d.text)))
	}
	req = buildCodeReviewRequest(summary, commitLog, joinNamed(keptDiff), guide, joinNamed(keptExc2))
	if len(req) <= maxUserInputLen {
		return req, truncateWarning(origLen, req, warns)
	}

	// Stage 4: 兜底：diff 保留末尾截断（新提交在后），再不行去掉 guide、硬截断
	overhead := len(buildCodeReviewRequest(summary, commitLog, "", guide, ""))
	maxPatchLen := maxUserInputLen - overhead
	if maxPatchLen < 100 {
		maxPatchLen = 100 // 绝不低于 100 字符
	}
	p := joinNamed(keptDiff)
	if len(p) > maxPatchLen {
		truncated := len(p) - maxPatchLen
		p = "..[diff 截断 " + strconv.Itoa(truncated) + " 字符].." + p[len(p)-maxPatchLen:]
		warns = append(warns, "diff 已截断（保留末尾）")
	}
	req = buildCodeReviewRequest(summary, commitLog, p, guide, joinNamed(keptExc2))
	if len(req) > maxUserInputLen && guide != "" {
		guide = ""
		warns = append(warns, "AGENTS.md 已移除（输入仍超限）")
		req = buildCodeReviewRequest(summary, commitLog, p, "", joinNamed(keptExc2))
	}
	if len(req) > maxUserInputLen {
		req = req[:maxUserInputLen] + "\n..[输入仍超限，已硬截断]..\n"
		warns = append(warns, "输入仍超限，已硬截断")
	}
	return req, truncateWarning(origLen, req, warns)
}

// dropUntilFits 按区段大小升序贪心保留（小文件优先，覆盖面最大），
// 返回保留与丢弃的区段。base 为必保内容，limit 为总预算。
func dropUntilFits(base string, sections []namedSection, limit int) (kept, dropped []namedSection) {
	sort.Slice(sections, func(i, j int) bool { return len(sections[i].text) < len(sections[j].text) })
	budget := limit - len(base)
	for _, s := range sections {
		if len(s.text) <= budget {
			kept = append(kept, s)
			budget -= len(s.text)
		} else {
			dropped = append(dropped, s)
		}
	}
	return kept, dropped
}

// joinNamed 拼接区段文本。
func joinNamed(secs []namedSection) string {
	var sb strings.Builder
	for _, s := range secs {
		sb.WriteString(s.text)
	}
	return sb.String()
}

// truncateWarning 生成截断告警：超限比例 + 截断动作列表。
func truncateWarning(origLen int, req string, warns []string) string {
	overLen := origLen - maxUserInputLen
	if overLen < 0 {
		overLen = 0
	}
	warning := fmt.Sprintf("⚠️ 审查输入过长（超出约 %d%%），已自动截断至 %d 字符。", overLen*100/maxUserInputLen, len(req))
	if len(warns) > 0 {
		warning += " " + strings.Join(warns, "；") + "。"
	}
	return warning
}
