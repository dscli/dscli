package issue

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// handleIssueShow 处理显示单个issue（Tool Calling）
func handleIssueShow(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return result, warning, err
	}

	issue, err := ShowIssue(ctx, number)
	if err != nil {
		return result, warning, err
	}

	// 构建详细结果
	var b strings.Builder
	b.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&b, "Issue #%s: %s\n", issue.Number, issue.Title)
	b.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&b, "ID:         %d\n", issue.ID)
	fmt.Fprintf(&b, "Number:     %s\n", issue.Number)
	fmt.Fprintf(&b, "State:      %s\n", issue.State)
	fmt.Fprintf(&b, "Created:    %s\n", formatTime(issue.CreatedAt))
	fmt.Fprintf(&b, "Updated:    %s\n", formatTime(issue.UpdatedAt))

	if !issue.ClosedAt.IsZero() {
		fmt.Fprintf(&b, "Closed:     %s\n", formatTime(issue.ClosedAt))
	}

	fmt.Fprintf(&b, "Author:     %s (%s)\n", issue.User.Name, issue.User.Login)

	assigneeInfo := "-"
	if issue.Assignee != nil {
		if issue.Assignee.Name != "" {
			assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
		} else {
			assigneeInfo = issue.Assignee.Login
		}
	}
	fmt.Fprintf(&b, "Assignee:   %s\n", assigneeInfo)

	labelsInfo := "-"
	if len(issue.Labels) > 0 {
		var labelNames []string
		for _, label := range issue.Labels {
			labelNames = append(labelNames, label.Name)
		}
		labelsInfo = strings.Join(labelNames, ", ")
	}
	fmt.Fprintf(&b, "Labels:     %s\n", labelsInfo)

	b.WriteString("\n" + strings.Repeat("-", 80) + "\n")
	b.WriteString("内容:\n")
	b.WriteString(strings.Repeat("-", 80) + "\n")

	if issue.Body != "" {
		b.WriteString(issue.Body + "\n")
	} else {
		b.WriteString("（无内容）\n")
	}

	b.WriteString(strings.Repeat("=", 80) + "\n")

	result = b.String()
	return result, warning, err
}

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_show",
		Description: "Show issue details.",
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