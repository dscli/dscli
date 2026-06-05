// Package issue to address issue create, list, show, assign, close
package issue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
)

// ==================== Issue 相关类型 ====================

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

// IssueAPIError 表示issue API调用错误
type IssueAPIError struct {
	StatusCode int
	Message    string
	Details    string
}

func (e *IssueAPIError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("issue API错误 (状态码: %d): %s\n详情: %s", e.StatusCode, e.Message, e.Details)
	}
	return fmt.Sprintf("issue API错误 (状态码: %d): %s", e.StatusCode, e.Message)
}

// IssueConfig 包含issue操作的配置信息
type IssueConfig struct {
	APIHost string
	BaseURL string
	Token   string
	Owner   string
	Repo    string // 仓库名称，用于GitCode API请求体
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
		outfmt.Println(strings.Repeat("=", 80))
		outfmt.Printf("Issue #%s: %s\n", issue.Number, issue.Title)
		outfmt.Println(strings.Repeat("=", 80))

		outfmt.Printf("ID:         %d\n", issue.ID)
		outfmt.Printf("Number:     %s\n", issue.Number)
		outfmt.Printf("State:      %s\n", issue.State)
		outfmt.Printf("Created:    %s\n", formatTime(issue.CreatedAt))
		outfmt.Printf("Updated:    %s\n", formatTime(issue.UpdatedAt))
		outfmt.Printf("Closed:     %s\n", formatTime(issue.ClosedAt))
		outfmt.Printf("Author:     %s (%s)\n", issue.User.Name, issue.User.Login)
		outfmt.Printf("Assignee:   %s\n", formatAssignee(issue.Assignee))
		outfmt.Printf("Labels:     %s\n", formatLabels(issue.Labels))

		outfmt.Println(strings.Repeat("-", 80))
		outfmt.Println("内容:")
		outfmt.Println(strings.Repeat("-", 80))
		if issue.Body != "" {
			outfmt.Println(issue.Body)
		} else {
			outfmt.Println("（无内容）")
		}
		outfmt.Println(strings.Repeat("=", 80))
	} else {
		// 简洁显示模式（用于list命令）
		assigneeInfo := formatAssignee(issue.Assignee)
		labelsInfo := formatLabels(issue.Labels)

		outfmt.Printf("#%s [%s] %s\n", issue.Number, issue.State, issue.Title)
		outfmt.Printf("  ID: %d | Author: %s | Assignee: %s\n",
			issue.ID, issue.User.Login, assigneeInfo)
		outfmt.Printf("  Created: %s | Updated: %s\n",
			formatTime(issue.CreatedAt), formatTime(issue.UpdatedAt))
		outfmt.Printf("  Labels: %s\n", labelsInfo)
		if issue.Body != "" {
			// 显示内容的前100个字符
			preview := issue.Body
			if len(preview) > 100 {
				preview = preview[:100] + "..."
			}
			outfmt.Printf("  Preview: %s\n", preview)
		}
		outfmt.Println()
	}
}

func IssueAPIBaseURL(originURL string, issueConfig *IssueConfig) (err error) {
	originURL = strings.TrimSpace(originURL)

	// 移除.git后缀
	originURL = strings.TrimSuffix(originURL, ".git")

	// 解析URL，支持SSH和HTTPS格式
	var host, owner, repo string

	if strings.HasPrefix(originURL, "git@") {
		// SSH格式: git@gitcode.com:nanjunjie/dscli
		parts := strings.Split(originURL, ":")
		if len(parts) != 2 {
			err = fmt.Errorf("invalid SSH URL format: %s", originURL)
			return err
		}
		host = strings.TrimPrefix(parts[0], "git@")
		path := parts[1]
		pathParts := strings.Split(path, "/")
		if len(pathParts) != 2 {
			err = fmt.Errorf("invalid path in SSH URL: %s", path)
			return err
		}
		owner = pathParts[0]
		repo = pathParts[1] // 需要repo参数用于请求体
	} else if strings.HasPrefix(originURL, "http") {
		// HTTPS格式: https://gitcode.com/nanjunjie/dscli
		// 移除协议前缀
		urlWithoutProtocol := strings.TrimPrefix(originURL, "https://")
		urlWithoutProtocol = strings.TrimPrefix(urlWithoutProtocol, "http://")

		parts := strings.Split(urlWithoutProtocol, "/")
		if len(parts) < 3 {
			err = fmt.Errorf("invalid HTTPS URL format: %s", originURL)
			return err
		}
		host = parts[0]
		owner = parts[1]
		repo = parts[2] // 需要repo参数用于请求体
	} else {
		err = fmt.Errorf("unsupported URL format: %s", originURL)
		return err
	}

	apiHost := map[string]string{
		"gitcode.com": "api.gitcode.com/api/v5",
	}[host]

	if apiHost == "" {
		err = fmt.Errorf("%s not support yet", host)
		return err
	}

	// 使用纯Go实现从.netrc获取token
	token, err := toolcall.GetTokenFromNetrc(host)
	if err != nil {
		return err
	}
	if token == "" {
		err = fmt.Errorf("no token found for %s in ~/.netrc", host)
		return err
	}
	issueConfig.APIHost = apiHost
	// 尝试使用/repos/:owner/:repo/issues格式
	issueConfig.BaseURL = fmt.Sprintf("https://%s/repos/%s", apiHost, owner)
	issueConfig.Token = token
	issueConfig.Owner = owner
	issueConfig.Repo = repo
	return err
}
