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
	fmt.Fprintf(&result, "Issue #%s: %s\n", issue.Number, issue.Title)
	result.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&result, "ID:         %d\n", issue.ID)
	fmt.Fprintf(&result, "Number:     %s\n", issue.Number)
	fmt.Fprintf(&result, "State:      %s\n", issue.State)
	fmt.Fprintf(&result, "Created:    %s\n", formatTime(issue.CreatedAt))
	fmt.Fprintf(&result, "Author:     %s (%s)\n", issue.User.Name, issue.User.Login)

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
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "issue标题（必需）,不能包含换行符，长度1-128字符",
					"pattern":     TitleLikePattern(128),
				},
				"body": map[string]any{
					"type":        "string",
					"description": "issue内容（可选），长度1-4096字符",
					"pattern":     ContentLikePattern(4096),
				},
			},
			"required":             []string{"title"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueCreate,
	})
}
