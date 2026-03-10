package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "issue_update",
		Description: "更新指定的issue",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "string",
					"description": "issue编号，必须是数字",
				},
				"title": map[string]any{
					"type":        "string",
					"description": "更新issue标题（可选）",
				},
				"body": map[string]any{
					"type":        "string",
					"description": "更新issue内容（可选）",
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
func handleIssueUpdate(ctx context.Context, args ToolArgs) (string, error) {
	number := ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	// 验证至少提供了一个更新字段
	title := ToolArgsValue(args, "title", "")
	body := ToolArgsValue(args, "body", "")
	state := ToolArgsValue(args, "state", "")

	if title == "" && body == "" && state == "" {
		return "", fmt.Errorf("必须提供至少一个更新字段（title, body 或 state）")
	}

	// 验证状态参数
	if state != "" && state != "open" && state != "closed" {
		return "", fmt.Errorf("状态必须是 'open' 或 'closed'，收到: %s", state)
	}

	issue, err := UpdateIssue(UpdateIssueOptions{
		Number: number,
		Title:  title,
		Body:   body,
		State:  state,
	})
	if err != nil {
		return "", err
	}

	// 构建成功结果
	var result strings.Builder
	result.WriteString("✅ Issue 更新成功!\n\n")

	result.WriteString(strings.Repeat("=", 80) + "\n")
	result.WriteString(fmt.Sprintf("Issue #%s: %s\n", issue.Number, issue.Title))
	result.WriteString(strings.Repeat("=", 80) + "\n\n")

	result.WriteString(fmt.Sprintf("ID:         %d\n", issue.ID))
	result.WriteString(fmt.Sprintf("Number:     %s\n", issue.Number))
	result.WriteString(fmt.Sprintf("State:      %s\n", issue.State))
	result.WriteString(fmt.Sprintf("Updated:    %s\n", formatTime(issue.UpdatedAt)))

	if title != "" {
		result.WriteString(fmt.Sprintf("标题已更新\n"))
	}
	if body != "" {
		result.WriteString(fmt.Sprintf("内容已更新\n"))
	}
	if state != "" {
		result.WriteString(fmt.Sprintf("状态已更新为: %s\n", state))
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")

	return result.String(), nil
}
