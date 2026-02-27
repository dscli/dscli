package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// printIssue 统一打印issue信息
func printIssue(issue Issue, detailed bool) {
	if detailed {
		// 详细显示模式（用于show命令）
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("Issue #%s: %s\n", issue.Number, issue.Title)
		fmt.Println(strings.Repeat("=", 80))

		fmt.Printf("ID:         %d\n", issue.ID)
		fmt.Printf("Number:     %s\n", issue.Number)
		fmt.Printf("State:      %s\n", issue.State)
		fmt.Printf("Created:    %s\n", formatTime(issue.CreatedAt))
		fmt.Printf("Updated:    %s\n", formatTime(issue.UpdatedAt))
		fmt.Printf("Closed:     %s\n", formatTime(issue.ClosedAt))
		fmt.Printf("Author:     %s (%s)\n", issue.User.Name, issue.User.Login)
		fmt.Printf("Assignee:   %s\n", formatAssignee(issue.Assignee))
		fmt.Printf("Labels:     %s\n", formatLabels(issue.Labels))

		fmt.Println(strings.Repeat("-", 80))
		fmt.Println("内容:")
		fmt.Println(strings.Repeat("-", 80))
		if issue.Body != "" {
			fmt.Println(issue.Body)
		} else {
			fmt.Println("（无内容）")
		}
		fmt.Println(strings.Repeat("=", 80))
	} else {
		// 简洁显示模式（用于list命令）
		assigneeInfo := formatAssignee(issue.Assignee)
		labelsInfo := formatLabels(issue.Labels)

		fmt.Printf("#%s [%s] %s\n", issue.Number, issue.State, issue.Title)
		fmt.Printf("  ID: %d | Author: %s | Assignee: %s\n",
			issue.ID, issue.User.Login, assigneeInfo)
		fmt.Printf("  Created: %s | Updated: %s\n",
			formatTime(issue.CreatedAt), formatTime(issue.UpdatedAt))
		fmt.Printf("  Labels: %s\n", labelsInfo)
		if issue.Body != "" {
			// 显示内容的前100个字符
			preview := issue.Body
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			fmt.Printf("  Preview: %s\n", preview)
		}
		fmt.Println()
	}
}

func init() {
	issueCmd := &cobra.Command{
		Use: "issue",
	}

	rootCmd.AddCommand(issueCmd)
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
			baseURL, token, err := IssueAPIBaseURL()
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
				printIssue(issue, false)
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
			baseURL, token, err := IssueAPIBaseURL()
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

			printIssue(issue, true)
			return nil
		},
	}

	createCmd := &cobra.Command{
		Use:  "create",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	updateCmd := &cobra.Command{
		Use:  "update",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	issueCmd.AddCommand(listCmd, showCmd, updateCmd, createCmd)
}

func IssueAPIBaseURL() (baseURL string, token string, err error) {
	originURL, err := ShellExec(`git remote get-url origin`)
	if err != nil {
		return
	}
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
