package ask

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

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
				"description": "Timeout in seconds (default 1200). The QA engineer may run multiple test rounds; set longer for large projects with many tests.",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "check",
	// QA runs go vet / go test, which can take longer than a review pass:
	// multiple test rounds plus a browser session per round.
	Timeout: 20 * time.Minute,
	Handler: handleQualityAssurance,
}

func init() {
	toolcall.RegisterTool(qualityAssuranceTool)
}

// handleQualityAssurance handles the quality assurance tool call.
func handleQualityAssurance(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleQualityAssurance")
	defer span.Finish()
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
	// demand via the DSML tool loop, and runs go vet / go test itself.
	structuredRequest, warning := truncateReviewRequest(summary, fullLog, patch)
	outfmt.Printf("📤 发送质量保障请求到 DeepSeek Web（免费 V4 Pro）...\n%s\n", structuredRequest)
	result, err = AskExpertWithRole(ctx, structuredRequest, "test")
	if err != nil {
		err = fmt.Errorf("质量保障失败: %w", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 质量保障报告\n%s\n", result)
	return result, warning, err
}
