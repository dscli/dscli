package ask

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/shell"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed quality_assurance.md
var quality_assurance_md string

var qualityAssuranceTool = toolcall.ToolDef{
	Name:        "quality_assurance",
	DisplayName: "Quality Assurance",
	Description: quality_assurance_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Required, release background and quality focus, 1-1024 chars",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Number of commits to assess, e.g. '-1' (last), '-2' (last 2), default '-1'",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds for the expert phase (default 0 = no extra bound; the tool-level budget is 30 minutes). The QA engineer may run multiple test rounds; set longer for large projects with many tests.",
			},
			"keep": map[string]any{
				"type":        "string",
				"description": "Continue a saved QA conversation (default new). Pass the conversation_id from a previous result, e.g. to resume a round interrupted mid tool-call: the pending tool calls are executed locally and their results fed back to the expert until it produces the final report.",
			},
		},
		// summary and keep are mutually exclusive modes: a new assessment
		// needs summary; resuming a saved conversation needs keep. Express
		// the conditional requirement with anyOf so a schema-validating
		// caller can pass one or the other (never both required).
		"anyOf": []any{
			map[string]any{"required": []string{"summary"}},
			map[string]any{"required": []string{"keep"}},
		},
		"additionalProperties": false,
	},
	Category: "check",
	// QA runs go vet / go test, which can take longer than a review pass:
	// multiple test rounds plus a browser session per round.
	Timeout: 30 * time.Minute,
	Handler: handleQualityAssurance,
}

func init() {
	toolcall.RegisterTool(qualityAssuranceTool)
}

// handleQualityAssurance handles the quality assurance tool call.
func handleQualityAssurance(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleQualityAssurance")
	defer span.Finish()

	// timeout bounds the expert phase (default 0 = no extra bound; the
	// tool-level Timeout is the ceiling). Wrapping the whole handler is
	// fine: the git checks finish in seconds.
	if secs := toolcall.ToolArgsValue(args, "timeout", 0); secs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(secs)*time.Second)
		defer cancel()
	}

	// keep=<conversation_id>: resume a saved QA conversation instead of
	// starting a new one. The summary/since/git checks are skipped — the
	// context the engineer needs is already in the conversation.
	if keep := toolcall.ToolArgsValue(args, "keep", ""); keep != "" {
		return resumeQualityAssurance(ctx, keep)
	}

	summary := toolcall.ToolArgsValue(args, "summary", "")
	since := toolcall.ToolArgsValue(args, "since", "-1")

	if strings.TrimSpace(summary) == "" {
		outfmt.Println("❌ 必须提供发布摘要")
		err = fmt.Errorf("必须提供发布摘要")
		return result, warning, err
	}

	if err := parseSince(since); err != nil {
		outfmt.Printf("❌ since 参数格式错误: %v\n", err)
		return result, warning, err
	}

	fmt.Println("🔍 检查是否有未提交的更改...")
	statusScript := `git status --porcelain | grep -v '^??'`
	status, shellErr := shell.SimpleExecute(ctx, statusScript)
	if shellErr != nil {
		status = ""
	}

	if status != "" {
		outfmt.Println("❌ 检测到未提交的更改")
		outfmt.Println("当前状态：")
		outfmt.Println(status)
		err = fmt.Errorf("请使用 'git status' 查看详情，并使用 'git add' 和 'git commit' 提交所有更改后再进行质量保障")
		return result, warning, err
	}

	outfmt.Println("✅ 没有未提交的更改")

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

	fullLogScript := `git log --format="%B" ` + since
	fullLog, err := shell.SimpleExecute(ctx, fullLogScript)
	if err != nil {
		fullLog = log
	}

	patchScript := fmt.Sprintf(`git --no-pager format-patch --stdout %s`, since)
	patch, err := shell.SimpleExecute(ctx, patchScript)
	if err != nil {
		fmt.Println("❌ 生成patch失败")
		err = fmt.Errorf("生成patch失败: %w", err)
		return result, warning, err
	}

	// The first message carries only the summary, commit message, and diff.
	// The QA engineer (role "test") reads AGENTS.md and file contents on
	// demand via the DSML tool loop, and runs go vet / go test itself —
	// provided the test role has tools configured (role_configs /
	// roles.DefaultFor: none by default). Without them the report is
	// limited to the diff itself; warn explicitly instead of degrading
	// silently (mirrors code_review).
	if doc := toolcall.BuildDSMLToolDoc(ctx, "test"); doc.Intro == "" {
		fmt.Fprintf(os.Stderr, "⚠️ test 角色未配置 DSML 工具（默认无工具）：QA 工程师将无法读取文件/运行 go vet/go test，报告限于提交内容。可运行 `dscli role update test --tools shell,read_file,apply_patch` 启用。\n")
	}
	structuredRequest, warning := truncateReviewRequest(summary, fullLog, patch)
	outfmt.Printf("📤 发送质量保障请求到 DeepSeek Web（免费 V4 Pro）...\n%s\n", structuredRequest)
	var convURL string
	result, convURL, err = AskExpertWithRoleConv(ctx, structuredRequest, "test")
	if err != nil {
		err = fmt.Errorf("质量保障失败: %w", err)
		return result, warning, err
	}

	// Surface the conversation ID so the caller can resume this exact QA
	// round later (keep=<id>) — e.g. after an interrupted tool-call round.
	if convID := lp.ConversationIDFromURL(convURL); convID != "" {
		outfmt.Printf("📋 keep:%s (继续此 QA 会话请传 keep=%s)\n", convID, convID)
		result += "\n\n---\nconversation_id: " + convID
	}

	outfmt.Printf("✅ 质量保障报告\n%s\n", result)
	return result, warning, err
}

// qaResumeFunc resumes a saved QA conversation. A package-level variable so
// tests can replace it with a mock (the real implementation drives Chrome via
// lp.HandleWebChatResume).
var qaResumeFunc = lp.HandleWebChatResume

// resumeQualityAssurance continues a saved QA conversation from its last
// assistant message — the web-chat twin of dscli chat's resume semantics.
// If that message ends with a tool-call block (the round was interrupted
// mid tool-call, e.g. a broken close tag that the old parser rejected), the
// pending calls are executed locally and their results fed back into the
// SAME conversation until the expert produces the final report. If the last
// message is a normal reply (multi-turn conversation), it is returned
// as-is — the caller decides whether a follow-up is needed.
func resumeQualityAssurance(ctx context.Context, keep string) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "resumeQualityAssurance")
	defer span.Finish()

	outfmt.Printf("🔁 恢复 QA 会话（keep=%s）...\n", keep)
	res, resumeErr := qaResumeFunc(ctx, lp.WebChatOptions{Keep: keep, Role: "test"})
	if resumeErr != nil {
		err = fmt.Errorf("质量保障恢复失败: %w", resumeErr)
		return result, warning, err
	}
	if res.Printed {
		// 工具循环已经逐轮打印过（含最终答案），调用方不要重复打印。
		outfmt.Println("✅ QA 会话恢复完成")
	} else {
		outfmt.Printf("✅ QA 会话恢复完成\n%s\n", res.Content)
	}
	return res.Content, "", nil
}
