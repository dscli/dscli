package ask

import (
	"context"
	_ "embed"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

// maxUserInputLen is the maximum length for the user portion of a code review
// request. DeepSeek's chat textarea enforces a total limit (~30k chars including
// the system prompt of ~2-3k), so we keep the user content under this threshold.
// 26000 keeps ~2.5k chars of margin under the combined limit.
const maxUserInputLen = 26000

// truncateReviewRequest 检查请求总长并在超限时按文件丢弃 diff 区段（最小优先）。
// 被丢弃文件的 diff 不再可见，但专家可用 read_file 读取对应文件补全，因此
// warning 明确列出被丢弃的文件（覆盖盲区）。
func truncateReviewRequest(summary, commitLog, patch string) (string, string) {
	req := buildCodeReviewRequest(summary, commitLog, patch)
	if len(req) <= maxUserInputLen {
		return req, ""
	}
	origLen := len(req)
	var warns []string

	// 按文件丢弃 diff 区段（小文件优先保留，保证覆盖面），专家可补读全文
	diffSecs := splitPatchByFile(patch)
	base := buildCodeReviewRequest(summary, commitLog, "")
	kept, dropped := dropUntilFits(base, diffSecs, maxUserInputLen)
	for _, d := range dropped {
		warns = append(warns, fmt.Sprintf("diff 过大，已丢弃 %s 的 diff（%d 字符），专家可用 read_file 读取该文件", d.name, len(d.text)))
	}
	req = buildCodeReviewRequest(summary, commitLog, joinNamed(kept))
	if len(req) <= maxUserInputLen {
		return req, truncateWarning(origLen, req, warns)
	}

	// 兜底：summary/commitLog 本身超限的极端情况，硬截断
	req = req[:maxUserInputLen] + "\n..[输入仍超限，已硬截断]..\n"
	warns = append(warns, "输入仍超限，已硬截断")
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
