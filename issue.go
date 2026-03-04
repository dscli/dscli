package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("%s?access_token=%s&state=%s", baseURL, token, state)
			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("API请求失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			// 先解析为RawIssue数组
			var rawIssues []RawIssue
			err = json.Unmarshal(b, &rawIssues)
			if err != nil {
				return fmt.Errorf("解析issue列表失败: %w", err)
			}

			// 转换为Issue数组
			issues, err := parseRawIssues(rawIssues)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
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
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}

			// 构建单个issue的API URL
			url := fmt.Sprintf("%s/%s?access_token=%s", baseURL, issueNumber, token)
			resp, err := http.Get(url)
			if err != nil {
				return fmt.Errorf("请求issue失败: %w", err)
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("API请求失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("读取响应失败: %w", err)
			}

			// 先解析为RawIssue
			var rawIssue RawIssue
			err = json.Unmarshal(b, &rawIssue)
			if err != nil {
				return fmt.Errorf("解析issue数据失败: %w", err)
			}

			// 转换为Issue
			issue, err := parseRawIssue(rawIssue)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
			}

			PrintIssue(issue, true)
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
				stat, _ := os.Stdin.Stat()
				if (stat.Mode() & os.ModeCharDevice) == 0 {
					// 有标准输入数据
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return fmt.Errorf("读取标准输入失败: %w", err)
					}
					body = string(data)
				}
			}

			// 获取API信息
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return err
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}

			// 准备请求数据
			requestData := map[string]any{
				"title": title,
			}
			if body != "" {
				requestData["body"] = body
			}

			// 转换为JSON
			jsonData, err := json.Marshal(requestData)
			if err != nil {
				return fmt.Errorf("序列化请求数据失败: %w", err)
			}

			// 发送POST请求
			url := fmt.Sprintf("%s?access_token=%s", baseURL, token)
			req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("发送请求失败: %w", err)
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("创建issue失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			// 解析响应
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("读取响应失败: %w", err)
			}

			// 解析为RawIssue
			var rawIssue RawIssue
			err = json.Unmarshal(b, &rawIssue)
			if err != nil {
				return fmt.Errorf("解析响应数据失败: %w", err)
			}

			// 转换为Issue
			issue, err := parseRawIssue(rawIssue)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
			}

			// 显示创建结果
			Println("✅ Issue 创建成功!")
			Println()
			PrintIssue(issue, true)
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

			// 获取API信息
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return err
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}

			// 准备请求数据
			requestData := make(map[string]any)
			if updateTitle != "" {
				requestData["title"] = updateTitle
			}
			if body != "" {
				requestData["body"] = body
			}
			if updateState != "" {
				// GitCode API 使用 "state_event" 而不是 "state"
				// 并且值应该是 "close" 而不是 "closed"
				if updateState == "closed" {
					requestData["state_event"] = "close"
				} else if updateState == "open" {
					requestData["state_event"] = "reopen"
				}
			}

			// 如果没有提供任何更新字段，直接返回
			if len(requestData) == 0 {
				return fmt.Errorf("没有提供有效的更新字段")
			}

			// 转换为JSON
			jsonData, err := json.Marshal(requestData)
			if err != nil {
				return fmt.Errorf("序列化请求数据失败: %w", err)
			}

			// 发送PATCH请求
			url := fmt.Sprintf("%s/%s?access_token=%s", baseURL, issueNumber, token)
			req, err := http.NewRequest("PATCH", url, strings.NewReader(string(jsonData)))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("发送请求失败: %w", err)
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("更新issue失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			// 解析响应
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("读取响应失败: %w", err)
			}

			// 解析为RawIssue
			var rawIssue RawIssue
			err = json.Unmarshal(b, &rawIssue)
			if err != nil {
				return fmt.Errorf("解析响应数据失败: %w", err)
			}

			// 转换为Issue
			issue, err := parseRawIssue(rawIssue)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
			}

			// 显示更新结果
			Println("✅ Issue 更新成功!")
			Println()
			PrintIssue(issue, true)
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

			// 获取API信息
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return err
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}

			// 准备请求数据 - 关闭issue
			requestData := map[string]any{
				"state_event": "close",
			}

			// 转换为JSON
			jsonData, err := json.Marshal(requestData)
			if err != nil {
				return fmt.Errorf("序列化请求数据失败: %w", err)
			}

			// 发送PATCH请求
			url := fmt.Sprintf("%s/%s?access_token=%s", baseURL, issueNumber, token)
			req, err := http.NewRequest("PATCH", url, strings.NewReader(string(jsonData)))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("发送请求失败: %w", err)
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("关闭issue失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			// 解析响应
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("读取响应失败: %w", err)
			}

			// 解析为RawIssue
			var rawIssue RawIssue
			err = json.Unmarshal(b, &rawIssue)
			if err != nil {
				return fmt.Errorf("解析响应数据失败: %w", err)
			}

			// 转换为Issue
			issue, err := parseRawIssue(rawIssue)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
			}

			// 显示关闭结果
			Println("✅ Issue 已关闭!")
			Println()
			PrintIssue(issue, true)
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

			// 获取API信息
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return err
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}

			// 准备请求数据 - 重新打开issue
			requestData := map[string]any{
				"state_event": "reopen",
			}

			// 转换为JSON
			jsonData, err := json.Marshal(requestData)
			if err != nil {
				return fmt.Errorf("序列化请求数据失败: %w", err)
			}

			// 发送PATCH请求
			url := fmt.Sprintf("%s/%s?access_token=%s", baseURL, issueNumber, token)
			req, err := http.NewRequest("PATCH", url, strings.NewReader(string(jsonData)))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("发送请求失败: %w", err)
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("重新打开issue失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			// 解析响应
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("读取响应失败: %w", err)
			}

			// 解析为RawIssue
			var rawIssue RawIssue
			err = json.Unmarshal(b, &rawIssue)
			if err != nil {
				return fmt.Errorf("解析响应数据失败: %w", err)
			}

			// 转换为Issue
			issue, err := parseRawIssue(rawIssue)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
			}

			// 显示重新打开结果
			Println("✅ Issue 已重新打开!")
			Println()
			PrintIssue(issue, true)
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

			// 获取API信息
			originURL, err := ShellExec(cmd.Context(), `git remote get-url origin`)
			if err != nil {
				return err
			}

			baseURL, token, err := IssueAPIBaseURL(originURL)
			if err != nil {
				return err
			}

			// 首先需要获取用户的ID
			// 这里简化处理，假设username就是用户ID
			// 在实际应用中，可能需要先查询用户ID
			assigneeID := username

			// 准备请求数据 - 分配issue
			requestData := map[string]any{
				"assignee_ids": []string{assigneeID},
			}

			// 转换为JSON
			jsonData, err := json.Marshal(requestData)
			if err != nil {
				return fmt.Errorf("序列化请求数据失败: %w", err)
			}

			// 发送PUT请求（GitLab API使用PUT来更新assignee）
			url := fmt.Sprintf("%s/%s?access_token=%s", baseURL, issueNumber, token)
			req, err := http.NewRequest("PUT", url, strings.NewReader(string(jsonData)))
			if err != nil {
				return fmt.Errorf("创建请求失败: %w", err)
			}
			req.Header.Set("Content-Type", "application/json")

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return fmt.Errorf("发送请求失败: %w", err)
			}
			defer resp.Body.Close()

			// 检查HTTP状态码
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("分配issue失败 (状态码: %d): %s", resp.StatusCode, string(body))
			}

			// 解析响应
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return fmt.Errorf("读取响应失败: %w", err)
			}

			// 解析为RawIssue
			var rawIssue RawIssue
			err = json.Unmarshal(b, &rawIssue)
			if err != nil {
				return fmt.Errorf("解析响应数据失败: %w", err)
			}

			// 转换为Issue
			issue, err := parseRawIssue(rawIssue)
			if err != nil {
				return fmt.Errorf("处理issue数据失败: %w", err)
			}

			// 显示分配结果
			Println("✅ Issue 已分配给用户!")
			Println()
			PrintIssue(issue, true)
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
