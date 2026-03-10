package main

import (
	"context"
	"fmt"
)

// handleIssueClose 处理关闭issue（Tool Calling）
func handleIssueClose(ctx context.Context, args ToolArgs) (string, error) {
	number := ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := CloseIssue(number)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("✅ Issue #%s 已关闭!\n当前状态: %s", issue.Number, issue.State), nil
}

func init() {
	RegisterTool(ToolDef{
		Name:        "issue_close",
		Description: "关闭指定的issue",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "string",
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
