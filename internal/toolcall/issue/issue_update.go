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
					"pattern":     toolcall.TitleLikePattern(128),
				},
				"body": map[string]any{
					"type":        "string",
					"description": "更新issue内容（可选），长度1-4096字符",
					"pattern":     toolcall.ContentLikePattern(4096),
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
func handleIssueUpdate(ctx context.Context, args toolcall.ToolArgs) (output string, user string, err error) {
	number := toolcall.ToolArgsValue(args, "number", 0)
	if number == 0 {
		err = fmt.Errorf("必须提供issue编号")
		return
	}

	// 验证至少提供了一个更新字段
	title := toolcall.ToolArgsValue(args, "title", "")
	body := toolcall.ToolArgsValue(args, "body", "")
	state := toolcall.ToolArgsValue(args, "state", "")

	if title == "" && body == "" && state == "" {
		err = fmt.Errorf("必须提供至少一个更新字段（title, body 或 state）")
		return
	}

	// 验证状态参数
	if state != "" && state != "open" && state != "closed" {
		err = fmt.Errorf("状态必须是 'open' 或 'closed'，收到: %s", state)
		return
	}

	issue, err := UpdateIssue(ctx, UpdateIssueOptions{
		Number: number,
		Title:  title,
		Body:   body,
		State:  state,
	})
	if err != nil {
		return
	}

	// 构建成功结果
	var result strings.Builder
	result.WriteString("✅ Issue 更新成功!\n\n")

	result.WriteString(strings.Repeat("=", 80) + "\n")
	fmt.Fprintf(&result, "Issue #%s: %s\n", issue.Number, issue.Title)
	result.WriteString(strings.Repeat("=", 80) + "\n\n")

	fmt.Fprintf(&result, "ID:         %d\n", issue.ID)
	fmt.Fprintf(&result, "Number:     %s\n", issue.Number)
	fmt.Fprintf(&result, "State:      %s\n", issue.State)
	fmt.Fprintf(&result, "Updated:    %s\n", formatTime(issue.UpdatedAt))

	if title != "" {
		fmt.Fprintf(&result, "标题已更新\n")
	}
	if body != "" {
		fmt.Fprintf(&result, "内容已更新\n")
	}
	if state != "" {
		fmt.Fprintf(&result, "状态已更新为: %s\n", state)
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")
	output = result.String()
	return
}
