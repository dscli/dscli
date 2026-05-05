package issue

import (
	"context"
	_ "embed"
	"fmt"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed issue_reopen.md
var issue_reopen_md string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_reopen",
		Description: issue_reopen_md,
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
		Handler:  handleIssueReopen,
	})
}

// handleIssueReopen 处理重新打开issue（Tool Calling）
func handleIssueReopen(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return result, warning, err
	}

	issue, err := ReopenIssue(ctx, number)
	if err != nil {
		return result, warning, err
	}

	result = fmt.Sprintf("✅ Issue #%s 已重新打开!\n当前状态: %s", issue.Number, issue.State)
	return result, warning, err
}