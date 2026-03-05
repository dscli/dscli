package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// RawIssue 用于接收原始JSON数据
type RawIssue struct {
	ID        json.RawMessage `json:"id"`
	Number    string          `json:"number"`
	State     string          `json:"state"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	ClosedAt  string          `json:"closed_at"`
	Labels    []Label         `json:"labels"`
	Assignee  *RawUser        `json:"assignee"`
	User      RawUser         `json:"user"`
}

// RawUser 原始用户数据
type RawUser struct {
	ID        json.RawMessage `json:"id"`
	Login     string          `json:"login"`
	Name      string          `json:"name"`
	AvatarURL string          `json:"avatar_url"`
}

// Label 表示issue的标签
type Label struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// Issue 处理后的issue数据结构
type Issue struct {
	ID        int
	Number    string
	State     string
	Title     string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time
	Labels    []Label
	Assignee  *User
	User      User
}

// User 处理后的用户信息
type User struct {
	ID        int
	Login     string
	Name      string
	AvatarURL string
}

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

// handleIssueShow 处理显示单个issue（Tool Calling）
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

// handleIssueCreate 处理创建issue（Tool Calling）
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

// handleIssueUpdate 处理更新issue（Tool Calling）
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
		result.WriteString("标题已更新\n")
	}
	if hasBody && body != "" {
		result.WriteString("内容已更新\n")
	}
	if hasState && state != "" {
		result.WriteString(fmt.Sprintf("状态已更新为: %s\n", state))
	}

	result.WriteString(strings.Repeat("=", 80) + "\n")

	return result.String(), nil
}

// handleIssueClose 处理关闭issue（Tool Calling）
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

// handleIssueReopen 处理重新打开issue（Tool Calling）
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

// handleIssueAssign 处理分配issue（Tool Calling）
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

func init() {
	// create命令的变量定义
	var (
		title    string
		bodyFlag string
		fileFlag string
	)

	issueCmd := &cobra.Command{
		Use: "issue",
	}

	RootCmd.AddCommand(issueCmd)
	var state string
	listCmd := &cobra.Command{
		Use: "list",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			switch state {
			case "open", "closed", "all":
				return
			}
			err = fmt.Errorf("state:%s should be in open, closed or all", state)
			return
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			issues, err := ListIssues(state)
			if err != nil {
				return err
			}

			for _, issue := range issues {
				PrintIssue(issue, false)
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&state, "state", "open", "issue state in open, closed and all, default open")

	showCmd := &cobra.Command{
		Use:   "show <number>",
		Short: "显示指定编号的issue详情",
		Long: `显示指定编号的issue详情。

示例:
  dscli issue show 123   # 显示编号为123的issue
  dscli issue show 45    # 显示编号为45的issue`,
		Args: cobra.ExactArgs(1), // 必须且只能有一个参数
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			// 验证参数是否为有效的数字
			issueNumber := args[0]
			if _, err := strconv.Atoi(issueNumber); err != nil {
				return fmt.Errorf("issue编号必须是数字，收到: %s", issueNumber)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			issueNumber := args[0]
			issue, err := ShowIssue(issueNumber)
			if err != nil {
				return err
			}

			PrintIssue(*issue, true)
			return nil
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "创建新的issue",
		Long: `创建新的issue。

可以通过以下方式提供内容：
1. 使用 --title 和 --body 参数
2. 使用 --title 参数，内容从标准输入读取
3. 使用 --file 参数从文件读取内容

示例:
  dscli issue create --title "Bug报告" --body "发现了一个bug..."
  echo "详细描述" | dscli issue create --title "功能请求"
  dscli issue create --title "文档更新" --file README.md`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 获取标题
			if title == "" {
				return fmt.Errorf("必须提供标题，使用 --title 参数")
			}

			// 获取内容
			var body string
			if bodyFlag != "" {
				// 从 --body 参数获取
				body = bodyFlag
			} else if fileFlag != "" {
				// 从文件读取
				content, err := os.ReadFile(fileFlag)
				if err != nil {
					return fmt.Errorf("读取文件失败: %w", err)
				}
				body = string(content)
			} else {
				// 从标准输入读取
				body, _ = ReadBodyFromStdinOrFile("")
			}

			// 创建issue
			issue, err := CreateIssue(CreateIssueOptions{
				Title: title,
				Body:  body,
			})
			if err != nil {
				return err
			}

			// 显示创建结果
			Println("✅ Issue 创建成功!")
			Println()
			PrintIssue(*issue, true)
			return nil
		},
	}
	// 绑定create命令的flags
	createCmd.Flags().StringVarP(&title, "title", "t", "", "issue标题（必需）")
	createCmd.Flags().StringVarP(&bodyFlag, "body", "b", "", "issue内容")
	createCmd.Flags().StringVarP(&fileFlag, "file", "f", "", "从文件读取内容")
	createCmd.MarkFlagRequired("title")

	// update命令的变量定义
	var (
		updateTitle string
		updateBody  string
		updateState string
		updateFile  string
	)

	updateCmd := &cobra.Command{
		Use:   "update <number>",
		Short: "更新指定的issue",
		Long: `更新指定的issue。

可以通过以下方式更新内容：
1. 使用 --title 更新标题
2. 使用 --body 更新内容
3. 使用 --state 更新状态（open/closed）
4. 使用 --file 从文件读取内容

示例:
  dscli issue update 123 --title "新的标题"
  dscli issue update 123 --body "更新后的内容"
  dscli issue update 123 --state closed
  dscli issue update 123 --title "新标题" --body "新内容" --state open
  dscli issue update 123 --file README.md`,
		Args: cobra.ExactArgs(1), // 必须且只能有一个参数
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			// 验证参数是否为有效的数字
			issueNumber := args[0]
			if _, err := strconv.Atoi(issueNumber); err != nil {
				return fmt.Errorf("issue编号必须是数字，收到: %s", issueNumber)
			}

			// 验证至少提供了一个更新字段
			if updateTitle == "" && updateBody == "" && updateState == "" && updateFile == "" {
				return fmt.Errorf("必须提供至少一个更新字段（--title, --body, --state 或 --file）")
			}

			// 验证状态参数
			if updateState != "" && updateState != "open" && updateState != "closed" {
				return fmt.Errorf("状态必须是 'open' 或 'closed'，收到: %s", updateState)
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber := args[0]

			// 获取内容
			var body string
			if updateBody != "" {
				// 从 --body 参数获取
				body = updateBody
			} else if updateFile != "" {
				// 从文件读取
				content, err := os.ReadFile(updateFile)
				if err != nil {
					return fmt.Errorf("读取文件失败: %w", err)
				}
				body = string(content)
			}

			// 更新issue
			issue, err := UpdateIssue(UpdateIssueOptions{
				Number: issueNumber,
				Title:  updateTitle,
				Body:   body,
				State:  updateState,
			})
			if err != nil {
				return err
			}

			// 显示更新结果
			Println("✅ Issue 更新成功!")
			Println()
			PrintIssue(*issue, true)
			return nil
		},
	}
	// 绑定update命令的flags
	updateCmd.Flags().StringVarP(&updateTitle, "title", "t", "", "更新issue标题")
	updateCmd.Flags().StringVarP(&updateBody, "body", "b", "", "更新issue内容")
	updateCmd.Flags().StringVarP(&updateState, "state", "s", "", "更新issue状态（open/closed）")
	updateCmd.Flags().StringVarP(&updateFile, "file", "f", "", "从文件读取内容")

	// close命令
	closeCmd := &cobra.Command{
		Use:   "close <number>",
		Short: "关闭指定的issue",
		Long: `关闭指定的issue。

示例:
  dscli issue close 123`,
		Args: cobra.ExactArgs(1), // 必须且只能有一个参数
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			// 验证参数是否为有效的数字
			issueNumber := args[0]
			if _, err := strconv.Atoi(issueNumber); err != nil {
				return fmt.Errorf("issue编号必须是数字，收到: %s", issueNumber)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber := args[0]
			issue, err := CloseIssue(issueNumber)
			if err != nil {
				return err
			}

			// 显示关闭结果
			Println("✅ Issue 已关闭!")
			Println()
			PrintIssue(*issue, true)
			return nil
		},
	}

	// reopen命令
	reopenCmd := &cobra.Command{
		Use:   "reopen <number>",
		Short: "重新打开指定的issue",
		Long: `重新打开指定的issue。

示例:
  dscli issue reopen 123`,
		Args: cobra.ExactArgs(1), // 必须且只能有一个参数
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			// 验证参数是否为有效的数字
			issueNumber := args[0]
			if _, err := strconv.Atoi(issueNumber); err != nil {
				return fmt.Errorf("issue编号必须是数字，收到: %s", issueNumber)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber := args[0]
			issue, err := ReopenIssue(issueNumber)
			if err != nil {
				return err
			}

			// 显示重新打开结果
			Println("✅ Issue 已重新打开!")
			Println()
			PrintIssue(*issue, true)
			return nil
		},
	}

	// assign命令
	assignCmd := &cobra.Command{
		Use:   "assign <number> <username>",
		Short: "分配issue给指定用户",
		Long: `分配issue给指定用户。

示例:
  dscli issue assign 123 username`,
		Args: cobra.ExactArgs(2),
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			// 验证参数是否为有效的数字
			issueNumber := args[0]
			if _, err := strconv.Atoi(issueNumber); err != nil {
				return fmt.Errorf("issue编号必须是数字，收到: %s", issueNumber)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			issueNumber := args[0]
			username := args[1]
			issue, err := AssignIssue(issueNumber, username)
			if err != nil {
				return err
			}

			// 显示分配结果
			Println("✅ Issue 已分配给用户!")
			Println()
			PrintIssue(*issue, true)
			return nil
		},
	}

	issueCmd.AddCommand(listCmd, showCmd, updateCmd, createCmd, closeCmd, reopenCmd, assignCmd)
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
