package main

import (
	"context"
	"fmt"
	"strings"
)

// handleIssueCreate 处理创建issue（Tool Calling）
func handleIssueCreate(ctx context.Context, args ToolArgs) (string, error) {
	title := ToolArgsValue(args, "title", "")
	if title == "" {
		return "", fmt.Errorf("必须提供标题")
	}

	body := ToolArgsValue(args, "body", "")

	issue, err := CreateIssue(CreateIssueOptions{
		Title: title,
		Body:  body,
	})
	if err != nil {
		return "", err
	}

	// 构建成功结果
	var result strings.Builder
	result.WriteString("✅ Issue 创建成功!\n\n")

	result.WriteString(strings.Repeat("=", 80) + "\n")
	result.WriteString(fmt.Sprintf("Issue #%s: %s\n", issue.Number, issue.Title))
	result.WriteString(strings.Repeat("=", 80) + "\n\n")

	result.WriteString(fmt.Sprintf("ID:         %d\n", issue.ID))
	result.WriteString(fmt.Sprintf("Number:     %s\n", issue.Number))
	result.WriteString(fmt.Sprintf("State:      %s\n", issue.State))
	result.WriteString(fmt.Sprintf("Created:    %s\n", formatTime(issue.CreatedAt)))
	result.WriteString(fmt.Sprintf("Author:     %s (%s)\n", issue.User.Name, issue.User.Login))

	if issue.Body != "" {
		result.WriteString("\n" + strings.Repeat("-", 80) + "\n")
		result.WriteString("内容:\n")
		result.WriteString(strings.Repeat("-", 80) + "\n")
		result.WriteString(issue.Body + "\n")
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")

	return result.String(), nil
}

func init() {
	RegisterTool(ToolDef{
		Name:        "issue_create",
		Description: "创建新的issue",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "issue标题（必需）",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "issue内容（可选）",
				},
			},
			"required":             []string{"title"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueCreate,
	})
}
