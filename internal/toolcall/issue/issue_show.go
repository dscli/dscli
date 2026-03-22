package issue

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// handleIssueShow 处理显示单个issue（Tool Calling）
func handleIssueShow(ctx context.Context, args toolcall.ToolArgs) (string, error) {
	number := toolcall.ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := ShowIssue(ctx, number)
	if err != nil {
		return "", err
	}

	// 构建详细结果
	var result strings.Builder
	result.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&result, "Issue #%s: %s\n", issue.Number, issue.Title)
	result.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&result, "ID:         %d\n", issue.ID)
	fmt.Fprintf(&result, "Number:     %s\n", issue.Number)
	fmt.Fprintf(&result, "State:      %s\n", issue.State)
	fmt.Fprintf(&result, "Created:    %s\n", formatTime(issue.CreatedAt))
	fmt.Fprintf(&result, "Updated:    %s\n", formatTime(issue.UpdatedAt))

	if !issue.ClosedAt.IsZero() {
		fmt.Fprintf(&result, "Closed:     %s\n", formatTime(issue.ClosedAt))
	}

	fmt.Fprintf(&result, "Author:     %s (%s)\n", issue.User.Name, issue.User.Login)

	assigneeInfo := "-"
	if issue.Assignee != nil {
		if issue.Assignee.Name != "" {
			assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
		} else {
			assigneeInfo = issue.Assignee.Login
		}
	}
	fmt.Fprintf(&result, "Assignee:   %s\n", assigneeInfo)

	labelsInfo := "-"
	if len(issue.Labels) > 0 {
		var labelNames []string
		for _, label := range issue.Labels {
			labelNames = append(labelNames, label.Name)
		}
		labelsInfo = strings.Join(labelNames, ", ")
	}
	fmt.Fprintf(&result, "Labels:     %s\n", labelsInfo)

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
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_show",
		Description: "显示指定编号的issue详情",
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
		Handler:  handleIssueShow,
	})
}
