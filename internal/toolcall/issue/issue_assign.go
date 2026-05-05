package issue

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// handleIssueAssign 处理分配issue（Tool Calling）
func handleIssueAssign(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return result, warning, err
	}

	username := toolcall.ToolArgsValue(args, "username", "")
	if username == "" {
		err = fmt.Errorf("必须提供用户名")
		return result, warning, err
	}

	issue, err := AssignIssue(ctx, number, username)
	if err != nil {
		return result, warning, err
	}

	assigneeInfo := username
	if issue.Assignee != nil && issue.Assignee.Name != "" {
		assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
	}
	result = fmt.Sprintf("✅ Issue #%s 已分配给用户: %s", issue.Number, assigneeInfo)
	return result, warning, err
}

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_assign",
		Description: "Assign issue to a specific user.",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "integer",
					"description": "issue编号，必须是数字",
				},
				"username": map[string]any{
					"type":        "string",
					"description": "用户名",
				},
			},
			"required":             []string{"number", "username"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueAssign,
	})
}