package issue

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_reopen",
		Description: "重新打开指定的issue",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "integer",
					"description": "issue编号，必须是数字",
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
func handleIssueReopen(ctx context.Context, args toolcall.ToolArgs) (output string, user string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return
	}

	issue, err := ReopenIssue(ctx, number)
	if err != nil {
		return
	}

	output = fmt.Sprintf("✅ Issue #%s 已重新打开!\n当前状态: %s", issue.Number, issue.State)
	return
}
