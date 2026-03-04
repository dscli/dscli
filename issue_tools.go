package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// registerIssueTools 注册issue管理工具
func registerIssueTools() {
	// issue_list 工具
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

	// issue_show 工具
	RegisterTool(ToolDef{
		Name:        "issue_show",
		Description: "显示指定编号的issue详情",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "string",
					"description": "issue编号，必须是数字",
				},
			},
			"required":             []string{"number"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueShow,
	})

	// issue_create 工具
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

	// issue_update 工具
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

	// issue_close 工具
	RegisterTool(ToolDef{
		Name:        "issue_close",
		Description: "关闭指定的issue",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "string",
					"description": "issue编号，必须是数字",
				},
			},
			"required":             []string{"number"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueClose,
	})

	// issue_reopen 工具
	RegisterTool(ToolDef{
		Name:        "issue_reopen",
		Description: "重新打开指定的issue",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "string",
					"description": "issue编号，必须是数字",
				},
			},
			"required":             []string{"number"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueReopen,
	})

	// issue_assign 工具
	RegisterTool(ToolDef{
		Name:        "issue_assign",
		Description: "分配issue给指定用户",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"number": map[string]any{
					"type":        "string",
					"description": "issue编号，必须是数字",
				},
				"username": map[string]any{
					"type":        "string",
					"description": "用户名",
				},
			},
			"required":             []string{"number", "username"},
			"additionalProperties": false,
		},
		Category: "issue",
		Handler:  handleIssueAssign,
	})
}

// handleIssueList 处理issue列表查询
func handleIssueList(ctx context.Context, args map[string]string) (string, error) {
	state, ok := args["state"]
	if !ok || state == "" {
		state = "open"
	}

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

// handleIssueShow 处理显示单个issue
func handleIssueShow(ctx context.Context, args map[string]string) (string, error) {
	number, ok := args["number"]
	if !ok || number == "" {
		return "", fmt.Errorf("必须提供issue编号")
	}

	// 验证参数
	if _, err := strconv.Atoi(number); err != nil {
		return "", fmt.Errorf("issue编号必须是数字，收到: %s", number)
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

// handleIssueCreate 处理创建issue
func handleIssueCreate(ctx context.Context, args map[string]string) (string, error) {
	title, ok := args["title"]
	if !ok || title == "" {
		return "", fmt.Errorf("必须提供标题")
	}

	body, ok := args["body"]
	if !ok {
		body = ""
	}

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

// handleIssueUpdate 处理更新issue
func handleIssueUpdate(ctx context.Context, args map[string]string) (string, error) {
	number, ok := args["number"]
	if !ok || number == "" {
		return "", fmt.Errorf("必须提供issue编号")
	}

	// 验证至少提供了一个更新字段
	title, hasTitle := args["title"]
	body, hasBody := args["body"]
	state, hasState := args["state"]

	if !hasTitle && !hasBody && !hasState {
		return "", fmt.Errorf("必须提供至少一个更新字段（title, body 或 state）")
	}

	// 验证状态参数
	if hasState && state != "" && state != "open" && state != "closed" {
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

	if hasTitle && title != "" {
		result.WriteString(fmt.Sprintf("标题已更新\n"))
	}
	if hasBody && body != "" {
		result.WriteString(fmt.Sprintf("内容已更新\n"))
	}
	if hasState && state != "" {
		result.WriteString(fmt.Sprintf("状态已更新为: %s\n", state))
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")

	return result.String(), nil
}

// handleIssueClose 处理关闭issue
func handleIssueClose(ctx context.Context, args map[string]string) (string, error) {
	number, ok := args["number"]
	if !ok || number == "" {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := CloseIssue(number)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("✅ Issue #%s 已关闭!\n当前状态: %s", issue.Number, issue.State), nil
}

// handleIssueReopen 处理重新打开issue
func handleIssueReopen(ctx context.Context, args map[string]string) (string, error) {
	number, ok := args["number"]
	if !ok || number == "" {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := ReopenIssue(number)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("✅ Issue #%s 已重新打开!\n当前状态: %s", issue.Number, issue.State), nil
}

// handleIssueAssign 处理分配issue
func handleIssueAssign(ctx context.Context, args map[string]string) (string, error) {
	number, ok := args["number"]
	if !ok || number == "" {
		return "", fmt.Errorf("必须提供issue编号")
	}

	username, ok := args["username"]
	if !ok || username == "" {
		return "", fmt.Errorf("必须提供用户名")
	}

	issue, err := AssignIssue(number, username)
	if err != nil {
		return "", err
	}

	assigneeInfo := username
	if issue.Assignee != nil && issue.Assignee.Name != "" {
		assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
	}

	assigneeInfo = username
	if issue.Assignee != nil && issue.Assignee.Name != "" {
		assigneeInfo = fmt.Sprintf("%s (%s)", issue.Assignee.Name, issue.Assignee.Login)
	}

	return fmt.Sprintf("✅ Issue #%s 已分配给用户: %s", issue.Number, assigneeInfo), nil
}
