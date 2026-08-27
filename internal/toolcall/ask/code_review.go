package ask

import (
	"context"
	_ "embed"
	"fmt"
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
				"description": "Timeout in seconds for the expert phase (default 0 = no extra bound; the tool-level budget is 30 minutes). Set longer for very large projects with many tests.",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "check",
	// The expert may run multiple tool-call rounds (each needs a browser
	// session + a model reply), so the budget must cover a full loop, not
	// just a single WebChat exchange. 30 minutes: a review of a large diff
	// can take several rounds, and 15 minutes proved too short.
	Timeout: 30 * time.Minute,
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
	// timeout bounds the expert phase (default 0 = no extra bound; the
	// tool-level Timeout is the ceiling). Wrapping the whole handler keeps
	// git/test steps fast anyway — they finish in seconds.
	if secs := toolcall.ToolArgsValue(args, "timeout", 0); secs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(secs)*time.Second)
		defer cancel()
	}

	if summary == "" {
		outfmt.Println("❌ 必须提供提交摘要")
		err = fmt.Errorf("必须提供提交摘要")
		return result, warning, err
	}

	// 校验 since 格式
	if err := parseSince(since); err != nil {
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

	// 首次请求只带提交信息 + diff：review 专家在 WebChat 工具循环里有
	// read_file/exec_command（见 internal/prompt/review.md），需要改动文件
	// 全文或项目指南（AGENTS.md）时会自己读取，不再预先注入 - 避免输入
	// 预算被全文挤占，也让专家按需深读任意上下文。
	structuredRequest, warning := truncateReviewRequest(summary, fullLog, patch)
	outfmt.Printf("📤 发送代码审查请求到 DeepSeek Web（免费 V4 Pro）...\n%s\n", structuredRequest)
	result, err = AskExpertWithRole(ctx, structuredRequest, "review")
	if err != nil {
		err = fmt.Errorf("代码审查失败: %w", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 代码审查结果\n%s\n", result)
	return result, warning, err
}

// parseSince 校验 since 参数：必须是 "-N" 格式（如 "-1", "-2", "-3"）。
func parseSince(since string) error {
	if !strings.HasPrefix(since, "-") {
		return fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	n, err := strconv.Atoi(since[1:])
	if err != nil || n < 1 {
		return fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	return nil
}

// ---------- 请求构建 ----------

// buildCodeReviewRequest 组装审查请求。patch 为空时省略对应区段。
func buildCodeReviewRequest(summary, commitLog, patch string) string {
	req := "## Commit Background\n" + summary + "\n\n## Commit Message\n" + commitLog
	if patch != "" {
		req += "\n\n## Code Changes\n" + patch
	}
	return req
}

// ---------- patch 文件拆分 ----------

// namedSection 是带文件名的文本区段（用于逐文件保留/丢弃）。
type namedSection struct {
	name string
	text string
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
			// 先记录 --- a/ 的旧路径作为回退名：删除文件时 +++ 为
			// /dev/null（新增文件则 --- 为 /dev/null、+++ 正常覆盖）。
			if strings.HasPrefix(line, "--- a/") {
				cur.name = strings.TrimPrefix(line, "--- a/")
			}
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

// ---------- 截断管线 ----------

// maxUserInputLen is the maximum RUNE count for the user portion of a code
// review request. The site limit is a character count (~158k runes measured
// 2026-08-29: ASCII 162k pass, ~165k reject; byte count is NOT the metric —
// see lp.webChatMaxInputRunes). The role prompt (~7KB ≈ 2-7k runes) is
// prepended by HandleWebChat, so the user portion keeps a margin below the
// site cap: 140k runes ≈ 158k cap minus role/system prompt and the DSML
// feedback that follow-up rounds append. (26000 was the old "~30k chars"
// era value; the site grew, and a too-small cap silently dropped diff
// sections the expert could not see.)
const maxUserInputLen = 140000

// truncNoteBudget 为请求正文末尾的截断提示（被丢弃文件清单）预留的字符空间：
// 清单随请求发送，专家才能感知覆盖盲区并主动 read_file 补读；该预算从 diff
// 保留中扣除（牺牲少量 diff 换取盲区可见）。
const truncNoteBudget = 4096

// maxWarnList 限制 warning 中明细列表的条数，避免极端场景（数百文件）时
// 本地告警把终端撑爆；buildTruncNote 中用同一上限保证提示本身不超预算。
const maxWarnList = 20

// truncateReviewRequest 检查请求总长并在超限时按文件丢弃 diff 区段（最小优先）。
// 被丢弃文件的 diff 不再可见；为保证覆盖盲区可见，截断提示（含被丢弃文件清单）
// 会拼进请求正文随请求发送，专家据此用 read_file 补读；同一提示也作为 warning
// 返回给本地用户。
func truncateReviewRequest(summary, commitLog, patch string) (string, string) {
	req := buildCodeReviewRequest(summary, commitLog, patch)
	if countRunes(req) <= maxUserInputLen {
		return req, ""
	}
	origLen := countRunes(req)
	var warns []string

	// 按文件丢弃 diff 区段（小文件优先保留，保证覆盖面），并为截断提示预留预算
	diffSecs := splitPatchByFile(patch)
	base := buildCodeReviewRequest(summary, commitLog, "")
	kept, dropped := dropUntilFits(base, diffSecs, maxUserInputLen-truncNoteBudget)
	for _, d := range dropped {
		warns = append(warns, fmt.Sprintf("diff 过大，已丢弃 %s 的 diff（%d 字符），专家可用 read_file 读取该文件", d.name, countRunes(d.text)))
	}

	req = buildCodeReviewRequest(summary, commitLog, joinNamed(kept)) + buildTruncNote(dropped, false)
	if countRunes(req) <= maxUserInputLen {
		return req, truncateWarning(origLen, req, warns)
	}

	// 兜底：summary/commitLog 本身超限（或清单超预算）的极端情况，按 rune 边界
	// 硬截断。先为截断提示预留预算再切正文，提示承载"已硬截断"信号并列出
	// 被丢弃文件，保证专家在最需要提示的场景仍能看到盲区。
	hardNote := buildTruncNote(dropped, true)
	body := buildCodeReviewRequest(summary, commitLog, joinNamed(kept))
	req = cutToRuneLen(body, maxUserInputLen-countRunes(hardNote)) + hardNote
	warns = append(warns, "输入仍超限，已硬截断")
	return req, truncateWarning(origLen, req, warns)
}

// countRunes returns the rune count of s (the site limit is a character
// count — see lp.webChatMaxInputRunes).
func countRunes(s string) int {
	return utf8.RuneCountInString(s)
}

// buildTruncNote 生成随请求发送的截断提示：列出被丢弃的文件（覆盖盲区），
// 让专家知道哪些文件需要 read_file 补读。清单最多列 maxWarnList 条，
// 超出时压缩为"前 N 条 + …等共 M 个"，保证提示本身不超预算。
// hardTrunc 为 true 表示走到了硬截断兜底（提示更强调信号）。
func buildTruncNote(dropped []namedSection, hardTrunc bool) string {
	var sb strings.Builder
	sb.WriteString("\n\n## ⚠️ 审查输入截断\n")
	if hardTrunc {
		sb.WriteString("输入超限，已硬截断；以下文件的 diff 未包含在本请求中，请用 read_file 读取补全：\n")
	} else {
		sb.WriteString("以下文件因输入长度限制未包含 diff，请用 read_file 读取补全：\n")
	}
	if len(dropped) == 0 {
		sb.WriteString("（无 diff 区段可列出）\n")
		return sb.String()
	}
	shown := dropped
	if len(shown) > maxWarnList {
		shown = shown[:maxWarnList]
	}
	for _, d := range shown {
		sb.WriteString("- ")
		sb.WriteString(d.name)
		sb.WriteString("\n")
	}
	if len(dropped) > maxWarnList {
		fmt.Fprintf(&sb, "…等共 %d 个文件\n", len(dropped))
	}
	return sb.String()
}

// cutToRuneLen 返回 s 的前缀，runes 数不超过 maxRunes（字节预算对 rune
// 计数不适用：站点限制按字符数，见 maxUserInputLen）。负预算（硬截断时
// 截断提示本身超预算的防御场景）返回空，绝不 panic。
func cutToRuneLen(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if countRunes(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

// dropUntilFits 按区段 rune 数升序贪心保留（小文件优先，覆盖面最大），
// 返回保留与丢弃的区段。base 为必保内容，limit 为总预算（rune 数）。
func dropUntilFits(base string, sections []namedSection, limit int) (kept, dropped []namedSection) {
	sort.Slice(sections, func(i, j int) bool { return countRunes(sections[i].text) < countRunes(sections[j].text) })
	budget := limit - countRunes(base)
	for _, s := range sections {
		if countRunes(s.text) <= budget {
			kept = append(kept, s)
			budget -= countRunes(s.text)
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

// truncateWarning 生成截断告警：超限比例 + 截断动作列表（超出 maxWarnList
// 条时压缩为前几条 + 总数）。比例与截断后长度均按 rune 数（站点限制语义）。
func truncateWarning(origLen int, req string, warns []string) string {
	overLen := origLen - maxUserInputLen
	if overLen < 0 {
		overLen = 0
	}
	warning := fmt.Sprintf("⚠️ 审查输入过长（超出约 %d%%），已自动截断至 %d 字符。", overLen*100/maxUserInputLen, countRunes(req))
	if len(warns) > 0 {
		warning += " "
		if len(warns) > maxWarnList {
			warning += strings.Join(warns[:maxWarnList], "；") + fmt.Sprintf("；…等共 %d 个", len(warns))
		} else {
			warning += strings.Join(warns, "；")
		}
		warning += "。"
	}
	return warning
}
