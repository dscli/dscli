package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// parseRawIssue 将RawIssue转换为Issue
func parseRawIssue(raw RawIssue) (Issue, error) {
	var issue Issue

	// 解析ID
	if idStr := string(raw.ID); idStr != "" && idStr != "null" {
		if id, err := strconv.Atoi(strings.Trim(idStr, `"`)); err == nil {
			issue.ID = id
		}
	}

	issue.Number = raw.Number
	issue.State = raw.State
	issue.Title = raw.Title
	issue.Body = raw.Body

	// 解析时间
	if raw.CreatedAt != "" {
		if t, err := time.Parse(time.RFC3339, raw.CreatedAt); err == nil {
			issue.CreatedAt = t
		}
	}
	if raw.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, raw.UpdatedAt); err == nil {
			issue.UpdatedAt = t
		}
	}
	if raw.ClosedAt != "" {
		if t, err := time.Parse(time.RFC3339, raw.ClosedAt); err == nil {
			issue.ClosedAt = t
		}
	}

	// 复制标签
	issue.Labels = raw.Labels

	// 解析用户
	if raw.User.ID != nil {
		if idStr := string(raw.User.ID); idStr != "" && idStr != "null" {
			if id, err := strconv.Atoi(strings.Trim(idStr, `"`)); err == nil {
				issue.User.ID = id
			}
		}
	}
	issue.User.Login = raw.User.Login
	issue.User.Name = raw.User.Name
	issue.User.AvatarURL = raw.User.AvatarURL

	// 解析负责人
	if raw.Assignee != nil {
		assignee := &User{}
		if raw.Assignee.ID != nil {
			if idStr := string(raw.Assignee.ID); idStr != "" && idStr != "null" {
				if id, err := strconv.Atoi(strings.Trim(idStr, `"`)); err == nil {
					assignee.ID = id
				}
			}
		}
		assignee.Login = raw.Assignee.Login
		assignee.Name = raw.Assignee.Name
		assignee.AvatarURL = raw.Assignee.AvatarURL
		issue.Assignee = assignee
	}

	return issue, nil
}

// parseRawIssues 解析多个RawIssue
func parseRawIssues(raws []RawIssue) ([]Issue, error) {
	var issues []Issue
	for _, raw := range raws {
		issue, err := parseRawIssue(raw)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}
	return issues, nil
}

// formatTime 格式化时间显示
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Format("2006-01-02 15:04:05")
}

// ==================== Tool Calling 处理器 ====================

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

// handleIssueClose 处理关闭issue（Tool Calling）
func handleIssueClose(ctx context.Context, args ToolArgs) (string, error) {
	number := ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := CloseIssue(number)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("✅ Issue #%s 已关闭!\n当前状态: %s", issue.Number, issue.State), nil
}

// handleIssueReopen 处理重新打开issue（Tool Calling）
func handleIssueReopen(ctx context.Context, args ToolArgs) (string, error) {
	number := ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	issue, err := ReopenIssue(number)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("✅ Issue #%s 已重新打开!\n当前状态: %s", issue.Number, issue.State), nil
}

// handleIssueAssign 处理分配issue（Tool Calling）
func handleIssueAssign(ctx context.Context, args ToolArgs) (string, error) {
	number := ToolArgsValue(args, "number", 0)
	if number == 0 {
		return "", fmt.Errorf("必须提供issue编号")
	}

	username := ToolArgsValue(args, "username", "")
	if username == "" {
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

	return fmt.Sprintf("✅ Issue #%s 已分配给用户: %s", issue.Number, assigneeInfo), nil
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

// formatAssignee 格式化负责人显示
func formatAssignee(assignee *User) string {
	if assignee == nil {
		return "-"
	}
	if assignee.Name != "" {
		return fmt.Sprintf("%s (%s)", assignee.Name, assignee.Login)
	}
	return assignee.Login
}

// formatLabels 格式化标签显示
func formatLabels(labels []Label) string {
	if len(labels) == 0 {
		return "-"
	}
	var labelNames []string
	for _, label := range labels {
		labelNames = append(labelNames, label.Name)
	}
	return strings.Join(labelNames, ", ")
}

// PrintIssue 统一打印issue信息
func PrintIssue(issue Issue, detailed bool) {
	if detailed {
		// 详细显示模式（用于show命令）
		Println(strings.Repeat("=", 80))
		Printf("Issue #%s: %s\n", issue.Number, issue.Title)
		Println(strings.Repeat("=", 80))

		Printf("ID:         %d\n", issue.ID)
		Printf("Number:     %s\n", issue.Number)
		Printf("State:      %s\n", issue.State)
		Printf("Created:    %s\n", formatTime(issue.CreatedAt))
		Printf("Updated:    %s\n", formatTime(issue.UpdatedAt))
		Printf("Closed:     %s\n", formatTime(issue.ClosedAt))
		Printf("Author:     %s (%s)\n", issue.User.Name, issue.User.Login)
		Printf("Assignee:   %s\n", formatAssignee(issue.Assignee))
		Printf("Labels:     %s\n", formatLabels(issue.Labels))

		Println(strings.Repeat("-", 80))
		Println("内容:")
		Println(strings.Repeat("-", 80))
		if issue.Body != "" {
			Println(issue.Body)
		} else {
			Println("（无内容）")
		}
		Println(strings.Repeat("=", 80))
	} else {
		// 简洁显示模式（用于list命令）
		assigneeInfo := formatAssignee(issue.Assignee)
		labelsInfo := formatLabels(issue.Labels)

		Printf("#%s [%s] %s\n", issue.Number, issue.State, issue.Title)
		Printf("  ID: %d | Author: %s | Assignee: %s\n",
			issue.ID, issue.User.Login, assigneeInfo)
		Printf("  Created: %s | Updated: %s\n",
			formatTime(issue.CreatedAt), formatTime(issue.UpdatedAt))
		Printf("  Labels: %s\n", labelsInfo)
		if issue.Body != "" {
			// 显示内容的前100个字符
			preview := issue.Body
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			Printf("  Preview: %s\n", preview)
		}
		Println()
	}
}

func IssueAPIBaseURL(originURL string) (baseURL string, token string, err error) {
	originURL = strings.TrimSpace(originURL)

	// 移除.git后缀
	originURL = strings.TrimSuffix(originURL, ".git")

	// 解析URL，支持SSH和HTTPS格式
	var host, owner, repo string

	if strings.HasPrefix(originURL, "git@") {
		// SSH格式: git@gitcode.com:dscli/dscli
		parts := strings.Split(originURL, ":")
		if len(parts) != 2 {
			err = fmt.Errorf("invalid SSH URL format: %s", originURL)
			return
		}
		host = strings.TrimPrefix(parts[0], "git@")
		path := parts[1]
		pathParts := strings.Split(path, "/")
		if len(pathParts) != 2 {
			err = fmt.Errorf("invalid path in SSH URL: %s", path)
			return
		}
		owner, repo = pathParts[0], pathParts[1]
	} else if strings.HasPrefix(originURL, "http") {
		// HTTPS格式: https://gitcode.com/dscli/dscli
		// 移除协议前缀
		urlWithoutProtocol := strings.TrimPrefix(originURL, "https://")
		urlWithoutProtocol = strings.TrimPrefix(urlWithoutProtocol, "http://")

		parts := strings.Split(urlWithoutProtocol, "/")
		if len(parts) < 3 {
			err = fmt.Errorf("invalid HTTPS URL format: %s", originURL)
			return
		}
		host = parts[0]
		owner, repo = parts[1], parts[2]
	} else {
		err = fmt.Errorf("unsupported URL format: %s", originURL)
		return
	}

	apiHost := map[string]string{
		"gitcode.com": "api.gitcode.com/api/v5",
	}[host]

	if apiHost == "" {
		err = fmt.Errorf("%s not support yet", host)
		return
	}

	// 使用纯Go实现从.netrc获取token
	token, err = GetTokenFromNetrc(host)
	if err != nil {
		return
	}
	if token == "" {
		err = fmt.Errorf("no token found for %s in ~/.netrc", host)
		return
	}
	baseURL = fmt.Sprintf("https://%s/repos/%s/%s/issues",
		apiHost, owner, repo)
	return
}
