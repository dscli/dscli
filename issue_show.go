package main

import (
	"context"
	"fmt"
	"strings"
)

// handleIssueShow 处理显示单个issue（Tool Calling）
func handleIssueShow(ctx context.Context, args ToolArgs) (string, error) {
	number := ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := ShowIssue(number)
	if err != nil {
		return "", err
	}

	// 构建详细结果
	var result strings.Builder
	result.WriteString(strings.Repeat("=", 80) + "\n")
	result.WriteString(fmt.Sprintf("Issue #%s: %s\n", issue.Number, issue.Title))
	result.WriteString(strings.Repeat("=", 80) + "\n\n")

	result.WriteString(fmt.Sprintf("ID:         %d\n", issue.ID))
	result.WriteString(fmt.Sprintf("Number:     %s\n", issue.Number))
	result.WriteString(fmt.Sprintf("State:      %s\n", issue.State))
	result.WriteString(fmt.Sprintf("Created:    %s\n", formatTime(issue.CreatedAt)))
	result.WriteString(fmt.Sprintf("Updated:    %s\n", formatTime(issue.UpdatedAt)))

	if !issue.ClosedAt.IsZero() {
		result.WriteString(fmt.Sprintf("Closed:     %s\n", formatTime(issue.ClosedAt)))
	}

	result.WriteString(fmt.Sprintf("Author:     %s (%s)\n", issue.User.Name, issue.User.Login))

	assigneeInfo := "-"
	if issue.Assignee != nil {
		if issue.Assignee.Name != "" {
			assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
		} else {
			assigneeInfo = issue.Assignee.Login
		}
	}
	result.WriteString(fmt.Sprintf("Assignee:   %s\n", assigneeInfo))

	labelsInfo := "-"
	if len(issue.Labels) > 0 {
		var labelNames []string
		for _, label := range issue.Labels {
			labelNames = append(labelNames, label.Name)
		}
		labelsInfo = strings.Join(labelNames, ", ")
	}
	result.WriteString(fmt.Sprintf("Labels:     %s\n", labelsInfo))

	result.WriteString("\n" + strings.Repeat("-", 80) + "\n")
	result.WriteString("内容:\n")
	result.WriteString(strings.Repeat("-", 80) + "\n")

	if issue.Body != "" {
		result.WriteString(issue.Body + "\n")
	} else {
		result.WriteString("（无内容）\n")
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")

	return result.String(), nil
}

func init() {
	RegisterTool(ToolDef{
		Name:        "issue_show",
		Description: "显示指定编号的issue详情",
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
		Handler:  handleIssueShow,
	})
}
