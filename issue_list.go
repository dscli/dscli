package main

import (
	"context"
	"fmt"
	"strings"
)

// handleIssueList 处理issue列表查询（Tool Calling）
func handleIssueList(ctx context.Context, args ToolArgs) (string, error) {
	state := ToolArgsValue(args, "state", "open")

	// 验证状态参数
	if state != "open" && state != "closed" && state != "all" {
		return "", fmt.Errorf("状态必须是 'open'、'closed' 或 'all'，收到: %s", state)
	}

	issues, err := ListIssues(state)
	if err != nil {
		return "", err
	}

	if len(issues) == 0 {
		return fmt.Sprintf("没有找到状态为 '%s' 的issues", state), nil
	}

	// 构建结果
	var result strings.Builder
	result.WriteString(fmt.Sprintf("📋 Issues (状态: %s, 总数: %d):\n\n", state, len(issues)))

	for _, issue := range issues {
		assigneeInfo := "-"
		if issue.Assignee != nil {
			if issue.Assignee.Name != "" {
				assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
			} else {
				assigneeInfo = issue.Assignee.Login
			}
		}

		labelsInfo := "-"
		if len(issue.Labels) > 0 {
			var labelNames []string
			for _, label := range issue.Labels {
				labelNames = append(labelNames, label.Name)
			}
			labelsInfo = strings.Join(labelNames, ", ")
		}

		result.WriteString(fmt.Sprintf("## #%s [%s] %s\n", issue.Number, issue.State, issue.Title))
		result.WriteString(fmt.Sprintf("  ID: %d | 作者: %s | 负责人: %s\n",
			issue.ID, issue.User.Login, assigneeInfo))
		result.WriteString(fmt.Sprintf("  创建时间: %s | 更新时间: %s\n",
			formatTime(issue.CreatedAt), formatTime(issue.UpdatedAt)))
		result.WriteString(fmt.Sprintf("  标签: %s\n", labelsInfo))

		if issue.Body != "" {
			preview := issue.Body
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			result.WriteString(fmt.Sprintf("  预览: %s\n", preview))
		}
		result.WriteString("\n")
	}

	return result.String(), nil
}

func init() {
	// 注册issue相关工具
	RegisterTool(ToolDef{
		Name:        "issue_list",
		Description: "列出项目中的issues，支持按状态过滤",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"state": map[string]any{
					"type":        "string",
					"description": "issue状态：open（打开）、closed（关闭）、all（全部），默认为open",
					"enum":        []string{"open", "closed", "all"},
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueList,
	})
}
