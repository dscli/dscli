package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestFormatFunctions 测试格式化辅助函数
func TestFormatFunctions(t *testing.T) {
	// 测试formatTime
	now := time.Now()
	formatted := formatTime(now)
	if formatted == "" {
		t.Error("formatTime返回空字符串")
	}
	if formatted == "-" {
		t.Error("有效时间不应该返回'-'")
	}

	// 测试空时间
	emptyTime := time.Time{}
	emptyFormatted := formatTime(emptyTime)
	if emptyFormatted != "-" {
		t.Errorf("空时间应该返回'-'，实际返回: %s", emptyFormatted)
	}

	// 测试formatAssignee
	assignee := &User{
		ID:    1,
		Login: "testuser",
		Name:  "测试用户",
	}
	assigneeStr := formatAssignee(assignee)
	expected := "测试用户 (testuser)"
	if assigneeStr != expected {
		t.Errorf("formatAssignee错误: 期望=%s, 实际=%s", expected, assigneeStr)
	}

	// 测试只有login的情况
	assigneeOnlyLogin := &User{
		ID:    2,
		Login: "anotheruser",
		Name:  "",
	}
	assigneeOnlyLoginStr := formatAssignee(assigneeOnlyLogin)
	if assigneeOnlyLoginStr != "anotheruser" {
		t.Errorf("只有login的formatAssignee错误: 期望=anotheruser, 实际=%s", assigneeOnlyLoginStr)
	}

	// 测试nil负责人
	nilAssigneeStr := formatAssignee(nil)
	if nilAssigneeStr != "-" {
		t.Errorf("nil负责人应该返回'-'，实际返回: %s", nilAssigneeStr)
	}

	// 测试formatLabels
	labels := []Label{
		{ID: 1, Name: "bug", Color: "red", Description: "Bug报告"},
		{ID: 2, Name: "enhancement", Color: "blue", Description: "功能增强"},
		{ID: 3, Name: "documentation", Color: "green", Description: "文档更新"},
	}
	labelsStr := formatLabels(labels)
	expectedLabels := "bug, enhancement, documentation"
	if labelsStr != expectedLabels {
		t.Errorf("formatLabels错误: 期望=%s, 实际=%s", expectedLabels, labelsStr)
	}

	// 测试空标签
	emptyLabelsStr := formatLabels([]Label{})
	if emptyLabelsStr != "-" {
		t.Errorf("空标签应该返回'-'，实际返回: %s", emptyLabelsStr)
	}

	// 测试单个标签
	singleLabel := []Label{
		{ID: 1, Name: "bug", Color: "red", Description: "Bug报告"},
	}
	singleLabelStr := formatLabels(singleLabel)
	if singleLabelStr != "bug" {
		t.Errorf("单个标签错误: 期望=bug, 实际=%s", singleLabelStr)
	}
}

// TestIssueStructs 测试Issue相关结构体
func TestIssueStructs(t *testing.T) {
	// 测试Label结构体
	label := Label{
		ID:          1,
		Name:        "bug",
		Color:       "#d73a4a",
		Description: "Bug报告",
	}
	if label.ID != 1 {
		t.Errorf("Label.ID错误: 期望=1, 实际=%d", label.ID)
	}
	if label.Name != "bug" {
		t.Errorf("Label.Name错误: 期望=bug, 实际=%s", label.Name)
	}
	if label.Color != "#d73a4a" {
		t.Errorf("Label.Color错误: 期望=#d73a4a, 实际=%s", label.Color)
	}

	// 测试User结构体
	user := User{
		ID:        1001,
		Login:     "octocat",
		Name:      "Octo Cat",
		AvatarURL: "https://avatar.url/octocat.png",
	}
	if user.ID != 1001 {
		t.Errorf("User.ID错误: 期望=1001, 实际=%d", user.ID)
	}
	if user.Login != "octocat" {
		t.Errorf("User.Login错误: 期望=octocat, 实际=%s", user.Login)
	}
	if user.Name != "Octo Cat" {
		t.Errorf("User.Name错误: 期望=Octo Cat, 实际=%s", user.Name)
	}

	// 测试Issue结构体
	issue := Issue{
		ID:        12345,
		Number:    "42",
		State:     "open",
		Title:     "测试Issue标题",
		Body:      "测试Issue内容",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		User:      user,
		Labels:    []Label{label},
	}
	if issue.ID != 12345 {
		t.Errorf("Issue.ID错误: 期望=12345, 实际=%d", issue.ID)
	}
	if issue.Number != "42" {
		t.Errorf("Issue.Number错误: 期望=42, 实际=%s", issue.Number)
	}
	if issue.State != "open" {
		t.Errorf("Issue.State错误: 期望=open, 实际=%s", issue.State)
	}
	if issue.Title != "测试Issue标题" {
		t.Errorf("Issue.Title错误: 期望=测试Issue标题, 实际=%s", issue.Title)
	}
	if len(issue.Labels) != 1 {
		t.Errorf("Issue.Labels长度错误: 期望=1, 实际=%d", len(issue.Labels))
	}
}

// TestTimeParsing 测试时间解析逻辑
func TestTimeParsing(t *testing.T) {
	// 测试RFC3339时间解析
	timeStr := "2024-01-01T12:00:00Z"
	parsedTime, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		t.Errorf("时间解析失败: %v", err)
	}
	if parsedTime.Year() != 2024 {
		t.Errorf("解析的年份错误: 期望=2024, 实际=%d", parsedTime.Year())
	}
	if parsedTime.Month() != time.January {
		t.Errorf("解析的月份错误: 期望=January, 实际=%v", parsedTime.Month())
	}
	if parsedTime.Day() != 1 {
		t.Errorf("解析的日期错误: 期望=1, 实际=%d", parsedTime.Day())
	}

	// 测试空字符串
	emptyTimeStr := ""
	_, err = time.Parse(time.RFC3339, emptyTimeStr)
	if err == nil {
		t.Error("空字符串时间解析应该失败")
	}

	// 测试无效格式
	invalidTimeStr := "invalid-time-format"
	_, err = time.Parse(time.RFC3339, invalidTimeStr)
	if err == nil {
		t.Error("无效格式时间解析应该失败")
	}
}

// TestPrintIssue 间接测试printIssue函数的相关逻辑
func TestPrintIssue(t *testing.T) {
	// 创建测试Issue
	issue := Issue{
		ID:        999,
		Number:    "99",
		State:     "closed",
		Title:     "已关闭的Issue",
		Body:      "这个Issue已经被关闭",
		CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
		ClosedAt:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
		User: User{
			ID:    888,
			Login: "closer",
			Name:  "关闭者",
		},
		Labels: []Label{
			{ID: 1, Name: "bug", Color: "red", Description: ""},
			{ID: 2, Name: "fixed", Color: "green", Description: ""},
		},
	}
	tcs := []struct {
		issue    Issue
		detailed bool
		result   string
	}{
		{issue, true, `================================================================================
Issue #99: 已关闭的Issue
================================================================================
ID:         999
Number:     99
State:      closed
Created:    2024-01-01 00:00:00
Updated:    2024-01-02 00:00:00
Closed:     2024-01-03 00:00:00
Author:     关闭者 (closer)
Assignee:   -
Labels:     bug, fixed
--------------------------------------------------------------------------------
内容:
--------------------------------------------------------------------------------
这个Issue已经被关闭
================================================================================`},
		{issue, false, `#99 [closed] 已关闭的Issue
  ID: 999 | Author: closer | Assignee: -
  Created: 2024-01-01 00:00:00 | Updated: 2024-01-02 00:00:00
  Labels: bug, fixed
  Preview: 这个Issue已经被关闭`},
	}

	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			buf := bytes.NewBuffer([]byte{})
			defer func() {
				Println = fmt.Println
				Printf = fmt.Printf
			}()

			Println = func(a ...any) (n int, err error) {
				return fmt.Fprintln(buf, a...)
			}
			Printf = func(format string, a ...any) (n int, err error) {
				return fmt.Fprintf(buf, format, a...)
			}
			PrintIssue(tc.issue, tc.detailed)
			result := strings.TrimSpace(buf.String())
			if result != tc.result {
				t.Log(result)
				t.Log(tc.result)
				t.Fatal()
			}
		})
	}
	// // 测试各个格式化函数
	// createdStr := formatTime(issue.CreatedAt)
	// if !strings.Contains(createdStr, "2024-01-01") {
	// 	t.Errorf("创建时间格式化错误: %s", createdStr)
	// }

	// updatedStr := formatTime(issue.UpdatedAt)
	// if !strings.Contains(updatedStr, "2024-01-02") {
	// 	t.Errorf("更新时间格式化错误: %s", updatedStr)
	// }

	// closedStr := formatTime(issue.ClosedAt)
	// if !strings.Contains(closedStr, "2024-01-03") {
	// 	t.Errorf("关闭时间格式化错误: %s", closedStr)
	// }

	// labelsStr := formatLabels(issue.Labels)
	// if labelsStr != "bug, fixed" {
	// 	t.Errorf("标签格式化错误: 期望=bug, fixed, 实际=%s", labelsStr)
	// }

	// // 验证Issue的基本信息
	// if issue.ID != 999 {
	// 	t.Errorf("Issue.ID错误: 期望=999, 实际=%d", issue.ID)
	// }
	// if issue.Number != "99" {
	// 	t.Errorf("Issue.Number错误: 期望=99, 实际=%s", issue.Number)
	// }
	// if issue.State != "closed" {
	// 	t.Errorf("Issue.State错误: 期望=closed, 实际=%s", issue.State)
	// }
}

// TestURLParsingLogic 测试URL解析逻辑
func TestURLParsingLogic(t *testing.T) {
	testCases := []struct {
		name        string
		url         string
		expectHost  string
		expectOwner string
		expectRepo  string
		shouldError bool
	}{
		{
			name:        "SSH格式",
			url:         "git@gitcode.com:owner/repo.git",
			expectHost:  "gitcode.com",
			expectOwner: "owner",
			expectRepo:  "repo",
			shouldError: false,
		},
		{
			name:        "HTTPS格式",
			url:         "https://gitcode.com/owner/repo.git",
			expectHost:  "gitcode.com",
			expectOwner: "owner",
			expectRepo:  "repo",
			shouldError: false,
		},
		{
			name:        "无.git后缀",
			url:         "https://gitcode.com/owner/repo",
			expectHost:  "gitcode.com",
			expectOwner: "owner",
			expectRepo:  "repo",
			shouldError: false,
		},
		{
			name:        "无效SSH格式",
			url:         "git@gitcode.com",
			shouldError: true,
		},
		{
			name:        "无效HTTPS格式",
			url:         "https://gitcode.com",
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟IssueAPIBaseURL中的URL处理逻辑
			url := strings.TrimSpace(tc.url)
			url = strings.TrimSuffix(url, ".git")

			var host, owner, repo string

			if strings.HasPrefix(url, "git@") {
				parts := strings.Split(url, ":")
				if len(parts) != 2 {
					// 应该出错
					if !tc.shouldError {
						t.Errorf("SSH格式解析应该成功: %s", url)
					}
					return
				}
				host = strings.TrimPrefix(parts[0], "git@")
				pathParts := strings.Split(parts[1], "/")
				if len(pathParts) != 2 {
					// 应该出错
					if !tc.shouldError {
						t.Errorf("SSH路径解析应该成功: %s", parts[1])
					}
					return
				}
				owner, repo = pathParts[0], pathParts[1]
			} else if strings.HasPrefix(url, "http") {
				// 移除协议
				noProtocol := strings.TrimPrefix(url, "https://")
				noProtocol = strings.TrimPrefix(noProtocol, "http://")

				parts := strings.Split(noProtocol, "/")
				if len(parts) < 3 {
					// 应该出错
					if !tc.shouldError {
						t.Errorf("HTTPS格式解析应该成功: %s", url)
					}
					return
				}
				host = parts[0]
				owner, repo = parts[1], parts[2]
			} else {
				// 不支持的格式
				if !tc.shouldError {
					t.Errorf("应该支持此格式: %s", url)
				}
				return
			}

			// 验证结果
			if tc.shouldError {
				t.Errorf("期望错误但解析成功: %s", url)
				return
			}

			if host != tc.expectHost {
				t.Errorf("host不匹配: 期望=%s, 实际=%s", tc.expectHost, host)
			}
			if owner != tc.expectOwner {
				t.Errorf("owner不匹配: 期望=%s, 实际=%s", tc.expectOwner, owner)
			}
			if repo != tc.expectRepo {
				t.Errorf("repo不匹配: 期望=%s, 实际=%s", tc.expectRepo, repo)
			}
		})
	}
}

// TestParseRawIssue 测试parseRawIssue函数
func TestParseRawIssue(t *testing.T) {
	testCases := []struct {
		name        string
		rawIssue    RawIssue
		expectError bool
		checkFunc   func(Issue) bool
	}{
		{
			name: "正常Issue解析",
			rawIssue: RawIssue{
				ID:        json.RawMessage(`"12345"`),
				Number:    "42",
				State:     "open",
				Title:     "测试Issue",
				Body:      "测试内容",
				CreatedAt: "2024-01-01T12:00:00Z",
				UpdatedAt: "2024-01-02T12:00:00Z",
				ClosedAt:  "",
				User: RawUser{
					ID:        json.RawMessage(`"67890"`),
					Login:     "testuser",
					Name:      "测试用户",
					AvatarURL: "https://avatar.url/test.png",
				},
				Labels: []Label{
					{ID: 1, Name: "bug", Color: "red", Description: "Bug报告"},
				},
			},
			expectError: false,
			checkFunc: func(issue Issue) bool {
				return issue.ID == 12345 &&
					issue.Number == "42" &&
					issue.State == "open" &&
					issue.Title == "测试Issue" &&
					len(issue.Labels) == 1
			},
		},
		{
			name: "有负责人的Issue",
			rawIssue: RawIssue{
				ID:     json.RawMessage(`"100"`),
				Number: "1",
				State:  "closed",
				Title:  "已关闭的Issue",
				User: RawUser{
					ID:    json.RawMessage(`"200"`),
					Login: "author",
				},
				Assignee: &RawUser{
					ID:    json.RawMessage(`"300"`),
					Login: "assignee",
					Name:  "负责人",
				},
			},
			expectError: false,
			checkFunc: func(issue Issue) bool {
				return issue.Assignee != nil &&
					issue.Assignee.Login == "assignee" &&
					issue.Assignee.Name == "负责人"
			},
		},
		{
			name: "无效ID格式",
			rawIssue: RawIssue{
				ID:     json.RawMessage(`"not-a-number"`),
				Number: "1",
				State:  "open",
				User: RawUser{
					ID:    json.RawMessage(`"200"`),
					Login: "user",
				},
			},
			expectError: false, // 函数应该处理错误，不返回错误
			checkFunc: func(issue Issue) bool {
				return issue.ID == 0 // ID应该为0，因为解析失败
			},
		},
		{
			name: "无效时间格式",
			rawIssue: RawIssue{
				ID:        json.RawMessage(`"100"`),
				Number:    "1",
				State:     "open",
				CreatedAt: "invalid-time",
				User: RawUser{
					ID:    json.RawMessage(`"200"`),
					Login: "user",
				},
			},
			expectError: false, // 时间解析失败不应该导致整个函数失败
			checkFunc: func(issue Issue) bool {
				return issue.CreatedAt.IsZero() // 时间应该为零值
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			issue, err := parseRawIssue(tc.rawIssue)

			if tc.expectError {
				if err == nil {
					t.Errorf("期望错误但得到成功")
				}
				return
			}

			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}

			if !tc.checkFunc(issue) {
				t.Errorf("检查函数返回false")
			}
		})
	}
}
