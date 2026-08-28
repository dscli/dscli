package ask

import (
	"context"
	_ "embed"
	"errors"
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

//go:embed code_dev.md
var code_dev_md string

var codeDevTool = toolcall.ToolDef{
	Name:        "code_dev",
	DisplayName: "Code Developer",
	Description: code_dev_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "The implementation task. A value starting with @ reads the task from a file (safe paths: cwd, ~/..., $HOME, or /tmp; max 1MB); otherwise sent as plain text.",
			},
			"keep": map[string]any{
				"type":        "string",
				"description": "Continue a previous developer conversation (default new). Pass the conversation_id from a previous result to send follow-up fix instructions to the SAME session (it keeps the full project context); keep-only resumes an interrupted round — pending tool calls are executed locally and results fed back.",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds for the developer phase (default 0 = no extra bound; the tool-level budget is 60 minutes). Set longer for large features with many test rounds.",
			},
		},
		// task and keep are partially interchangeable: a new implementation
		// needs task; resuming an interrupted session needs keep; sending
		// follow-up fixes to an existing session uses both. Express the
		// conditional requirement with anyOf (mirrors quality_assurance).
		"anyOf": []any{
			map[string]any{"required": []string{"task"}},
			map[string]any{"required": []string{"keep"}},
		},
		"additionalProperties": false,
	},
	Category: "check",
	// A development session runs many DSML tool-call rounds (implement →
	// test → iterate → commit), each round being a browser session plus a
	// model reply. code_review needs 30 min for a review pass; implementing
	// a feature with several test rounds is heavier: 60 min.
	Timeout: 60 * time.Minute,
	Handler: handleCodeDev,
}

func init() {
	// WebChat is always available (free DeepSeek V4 Pro) — no API key needed.
	toolcall.RegisterTool(codeDevTool)
}

// handleCodeDev implements the code_dev tool: it hands an implementation
// task to the built-in dev role via DeepSeek Web, and the dev works in the
// local repo through the DSML tool loop (role "dev" defaults to all tools,
// gated by the same role_configs / roles.DefaultFor source that gates
// GetAllTools).
func handleCodeDev(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleCodeDev")
	defer span.Finish()

	// timeout bounds the developer phase (default 0 = no extra bound; the
	// tool-level Timeout is the ceiling). git checks finish in seconds, so
	// wrapping the whole handler is fine.
	if secs := toolcall.ToolArgsValue(args, "timeout", 0); secs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(secs)*time.Second)
		defer cancel()
	}

	task := toolcall.ToolArgsValue(args, "task", "")
	keep := toolcall.ToolArgsValue(args, "keep", "")

	// keep-only mode: resume an interrupted developer session instead of
	// starting a new one. The task context the developer needs is already
	// in the conversation (mirrors quality_assurance / resumeQualityAssurance).
	if keep != "" && strings.TrimSpace(task) == "" {
		return resumeCodeDev(ctx, keep)
	}

	if strings.TrimSpace(task) == "" {
		outfmt.Println("❌ 必须提供实现任务 (task)，或传 keep 恢复会话")
		err = fmt.Errorf("必须提供实现任务 (task)")
		return result, warning, err
	}

	// An @-prefixed task is a file reference (e.g. @docs/architecture.md):
	// read it when it is a safe path and exists. Lenient fallback: anything
	// else is sent as plain text (mirrors ask_expert's input handling).
	content := task
	if strings.HasPrefix(task, "@") && len(task) > 1 {
		candidate := task[1:]
		if isSafePath(candidate) {
			fileContent, readErr := readContentFile(candidate)
			switch {
			case readErr == nil:
				content = fileContent
				outfmt.Printf("📂 Read task from file: %s (%d bytes)\n", candidate, len(content))
			case errors.Is(readErr, os.ErrNotExist):
				outfmt.Printf("⚠️  @%s not found, sending as plain text\n", candidate)
			default:
				err = readErr
				return result, warning, err
			}
		}
	}

	// Never silently fail on uncommitted changes: the developer may work on
	// top of a half-finished tree (that is legitimate), but the caller must
	// know the starting point — the delivery contract requires a clean tree
	// on return so code_review can see the new commit alone.
	statusScript := `git status --porcelain | grep -v '^??'`
	status, shellErr := shell.SimpleExecute(ctx, statusScript)
	switch {
	case shellErr != nil:
		outfmt.Println("⚠️  无法检查 git 状态（可能不在 git 仓库中）")
	case status != "":
		outfmt.Println("⚠️  工作区存在未提交更改，开发者将在其上继续工作；完成后所有更改都必须提交（code_review 需要干净工作区）。")
	default:
		outfmt.Println("✅ 工作区干净（或仅有未跟踪文件）")
	}

	// The dev role defaults to all tools (roles.DefaultFor), but a project
	// may have narrowed it via `dscli role update dev --tools ...`. Warn
	// explicitly when the role has no DSML tools instead of letting the
	// session silently degrade (mirrors code_review / quality_assurance).
	if doc := toolcall.BuildDSMLToolDoc(ctx, "dev"); doc.Intro == "" {
		fmt.Fprintf(os.Stderr, "⚠️ dev 角色未配置 DSML 工具（role update dev 缩小了工具集）：开发者将无法读取文件/执行命令。可运行 `dscli role reset dev` 恢复全部工具。\n")
	}

	// Compose the request: the task plus the delivery contract. AGENTS.md
	// and file contents are NOT injected — the developer reads them on
	// demand via read_file, keeping the request under the web-chat input
	// budget (the dev role prompt already carries project context).
	structuredRequest, warning := truncateCodeDevRequest(content)
	outfmt.Printf("📤 发送实现任务到 DeepSeek Web（免费 V4 Pro，角色 dev）...\n%s\n", truncateForDisplay(structuredRequest, 2000))

	// keep continues the SAME developer conversation ("" = new): follow-up
	// fix instructions keep the full project context the developer built up.
	reply, convURL, printed, err := askExpertWithRoleFunc(ctx, structuredRequest, "dev", "", "", keep, nil)
	if err != nil {
		outfmt.Println("❌ 开发会话失败")
		err = fmt.Errorf("开发会话失败: %w", err)
		return result, warning, err
	}
	result = strings.TrimSpace(reply)

	// Surface the conversation ID so the caller can send follow-ups to this
	// exact developer session later (keep=<id>).
	var convID string
	if convURL != "" {
		if convID = lp.ConversationIDFromURL(convURL); convID != "" {
			outfmt.Printf("📋 keep:%s (继续此开发会话请传 keep=%s)\n", convID, convID)
			result += "\n\n---\nconversation_id: " + convID
		} else {
			outfmt.Printf("📋 conversation URL: %s\n", convURL)
		}
	}

	if printed {
		outfmt.Println("✅ 开发会话完成")
	} else {
		outfmt.Printf("✅ 开发会话完成\n\n%s\n", result)
	}
	return result, warning, err
}

// devResumeFunc resumes a saved developer conversation. A package-level
// variable so tests can replace it with a mock (the real implementation
// drives Chrome via lp.HandleWebChatResume).
var devResumeFunc = lp.HandleWebChatResume

// resumeCodeDev continues a saved developer conversation from its last
// assistant message — the web-chat twin of dscli chat's resume semantics.
// If that message ends with a tool-call block (the round was interrupted
// mid tool-call), the pending calls are executed locally and their results
// fed back into the SAME conversation until the developer produces a final
// report. If the last message is a normal reply (multi-turn conversation),
// it is returned as-is — the caller decides whether a follow-up is needed.
func resumeCodeDev(ctx context.Context, keep string) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "resumeCodeDev")
	defer span.Finish()

	outfmt.Printf("🔁 恢复开发会话（keep=%s）...\n", keep)
	res, resumeErr := devResumeFunc(ctx, lp.WebChatOptions{Keep: keep, Role: "dev"})
	if resumeErr != nil {
		err = fmt.Errorf("开发会话恢复失败: %w", resumeErr)
		return result, warning, err
	}
	if res.Printed {
		// 工具循环已经逐轮打印过（含最终答案），调用方不要重复打印。
		outfmt.Println("✅ 开发会话恢复完成")
	} else {
		outfmt.Printf("✅ 开发会话恢复完成\n%s\n", res.Content)
	}
	return res.Content, "", nil
}

// ---------- 请求构建 ----------

// buildCodeDevRequest 组装实现请求：任务主体 + 交付契约。AGENTS.md 与
// 项目文件不注入（开发者用 read_file 按需读取，参见 code_review.go 的
// 同款设计）。
func buildCodeDevRequest(task string) string {
	return "## Implementation Task\n" + task + `

## Delivery Contract
- Implement the task above in the project repository.
- Read AGENTS.md first if present: it carries build instructions, architecture, and coding conventions.
- Run the project's test suite after implementing; fix any failures.
- Commit ALL changes with a descriptive English commit message (git add + git commit).
- The working tree must be clean when you finish — the next step (code_review) requires it.
- Final report: what was implemented, tests run and their outcome, commit hash, and any follow-up risks.
`
}

// truncateCodeDevRequest 检查请求总长并在超限时按 rune 边界硬截断。
// 站点输入上限是字符数（见 maxUserInputLen）；截断提示随请求发送，
// 让开发者知道任务被截断（缺口可用 read_file 补读的任务文案由调用方
// 负责——本地告警同样返回）。
func truncateCodeDevRequest(task string) (string, string) {
	req := buildCodeDevRequest(task)
	if countRunes(req) <= maxUserInputLen {
		return req, ""
	}
	origLen := countRunes(req)
	hardNote := "\n\n## ⚠️ 任务输入截断\n任务超出输入上限，已按字符边界截断；缺失部分请用 read_file 读取任务引用文件补全（如架构文档）。\n"
	req = cutToRuneLen(req, maxUserInputLen-countRunes(hardNote)) + hardNote
	warning := fmt.Sprintf("⚠️ 任务输入过长（超出约 %d%%），已自动截断至 %d 字符。", origLen*100/maxUserInputLen, countRunes(req))
	return req, warning
}
