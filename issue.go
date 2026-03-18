package main

import (
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

func IssueAPIBaseURL(originURL string) (baseURL string, token string, repo string, err error) {
	originURL = strings.TrimSpace(originURL)

	// 移除.git后缀
	originURL = strings.TrimSuffix(originURL, ".git")

	// 解析URL，支持SSH和HTTPS格式
	var host, owner string

	if strings.HasPrefix(originURL, "git@") {
		// SSH格式: git@gitcode.com:nanjunjie/dscli
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
			return
		}
		host = parts[0]
		owner = parts[1]
		repo = parts[2] // 需要repo参数用于请求体
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
	// GitCode API格式: /api/v5/repos/:owner/issues
	// 注意：URL中不包含repo，repo在请求体中
	baseURL = fmt.Sprintf("https://%s/repos/%s/issues",
		apiHost, owner)
	return
}
