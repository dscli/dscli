package issue

import (
	"context"
	_ "embed"
	"fmt"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed issue_close.md
var issue_close_md string

// handleIssueClose 处理关闭issue（Tool Calling）
func handleIssueClose(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return result, warning, err
	}

	issue, err := CloseIssue(ctx, number)
	if err != nil {
		return result, warning, err
	}

	result = fmt.Sprintf("✅ Issue #%s 已关闭!\n当前状态: %s", issue.Number, issue.State)
	return result, warning, err
}

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_close",
		Description: issue_close_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "integer",
					"description": "Issue number (required, must be a number)",
				},
			},
			"required":             []string{"number"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueClose,
	})
}