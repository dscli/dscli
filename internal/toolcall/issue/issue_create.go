package issue

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// handleIssueCreate 处理创建issue（Tool Calling）
func handleIssueCreate(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	title := toolcall.ToolArgsValue(args, "title", "")
	if title == "" {
		err = fmt.Errorf("必须提供标题")
		return result, warning, err
	}

	body := toolcall.ToolArgsValue(args, "body", "")

	issue, err := CreateIssue(ctx, CreateIssueOptions{
		Title: title,
		Body:  body,
	})
	if err != nil {
		return result, warning, err
	}

	// 构建成功结果
	var b strings.Builder
	b.WriteString("✅ Issue 创建成功!\n\n")

	b.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&b, "Issue #%s: %s\n", issue.Number, issue.Title)
	b.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&b, "ID:         %d\n", issue.ID)
	fmt.Fprintf(&b, "Number:     %s\n", issue.Number)
	fmt.Fprintf(&b, "State:      %s\n", issue.State)
	fmt.Fprintf(&b, "Created:    %s\n", formatTime(issue.CreatedAt))
	fmt.Fprintf(&b, "Author:     %s (%s)\n", issue.User.Name, issue.User.Login)

	if issue.Body != "" {
		b.WriteString("\n" + strings.Repeat("-", 80) + "\n")
		b.WriteString("内容:\n")
		b.WriteString(strings.Repeat("-", 80) + "\n")
		b.WriteString(issue.Body + "\n")
	}

	b.WriteString(strings.Repeat("=", 80) + "\n")

	result = b.String()
	return result, warning, err
}

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_create",
		Description: "创建新的issue",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title": map[string]any{
					"type":        "string",
					"description": "issue标题（必需）,不能包含换行符，长度1-128字符",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "issue内容（可选），长度1-4096字符",
				},
			},
			"required":             []string{"title"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueCreate,
	})
}
