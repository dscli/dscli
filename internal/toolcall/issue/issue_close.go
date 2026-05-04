package issue

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// handleIssueClose 处理关闭issue（Tool Calling）
func handleIssueClose(ctx context.Context, args toolcall.ToolArgs) (result string, warning string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return
	}

	issue, err := CloseIssue(ctx, number)
	if err != nil {
		return
	}

	result = fmt.Sprintf("✅ Issue #%s 已关闭!\n当前状态: %s", issue.Number, issue.State)
	return
}

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_close",
		Description: "关闭指定的issue",
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
		Handler:  handleIssueClose,
	})
}
