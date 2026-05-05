package issue

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "issue_update",
		Description: "更新指定的issue",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "integer",
					"description": "issue编号，必须是数字",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "更新issue标题（可选）,不可有回车，长度1-128字符",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "更新issue内容（可选），长度1-4096字符",
				},
				"state": map[string]any{
					"type":        "string",
					"description": "更新issue状态：open（打开）、closed（关闭）（可选）",
					"enum":        []string{"open", "closed"},
				},
			},
			"required":             []string{"number"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueUpdate,
	})
}

// handleIssueUpdate 处理更新issue（Tool Calling）
func handleIssueUpdate(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	number := int(toolcall.ToolArgsValue(args, "number", int64(0)))
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return result, warning, err
	}

	// 验证至少提供了一个更新字段
	title := toolcall.ToolArgsValue(args, "title", "")
	body := toolcall.ToolArgsValue(args, "body", "")
	state := toolcall.ToolArgsValue(args, "state", "")

	if title == "" && body == "" && state == "" {
		err = fmt.Errorf("必须提供至少一个更新字段（title, body 或 state）")
		return result, warning, err
	}

	// 验证状态参数
	if state != "" && state != "open" && state != "closed" {
		err = fmt.Errorf("状态必须是 'open' 或 'closed'，收到: %s", state)
		return result, warning, err
	}

	issue, err := UpdateIssue(ctx, UpdateIssueOptions{
		Number: number,
		Title:  title,
		Body:   body,
		State:  state,
	})
	if err != nil {
		return result, warning, err
	}

	// 构建成功结果
	var b strings.Builder
	b.WriteString("✅ Issue 更新成功!\n\n")

	b.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&b, "Issue #%s: %s\n", issue.Number, issue.Title)
	b.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&b, "ID:         %d\n", issue.ID)
	fmt.Fprintf(&b, "Number:     %s\n", issue.Number)
	fmt.Fprintf(&b, "State:      %s\n", issue.State)
	fmt.Fprintf(&b, "Updated:    %s\n", formatTime(issue.UpdatedAt))

	if title != "" {
		fmt.Fprintf(&b, "标题已更新\n")
	}
	if body != "" {
		fmt.Fprintf(&b, "内容已更新\n")
	}
	if state != "" {
		fmt.Fprintf(&b, "状态已更新为: %s\n", state)
	}

	b.WriteString(strings.Repeat("=", 80) + "\n")
	result = b.String()
	return result, warning, err
}
