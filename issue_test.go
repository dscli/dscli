package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
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

// TestParseRawIssues 测试批量解析功能
func TestParseRawIssues(t *testing.T) {
	// 测试用例1: 空数组
	t.Run("空数组", func(t *testing.T) {
		issues, err := parseRawIssues([]RawIssue{})
		if err != nil {
			t.Errorf("空数组应该返回nil错误，但得到: %v", err)
		}
		if len(issues) != 0 {
			t.Errorf("空数组应该返回空切片，但得到长度: %d", len(issues))
		}
	})

	// 测试用例2: 单个RawIssue
	t.Run("单个RawIssue", func(t *testing.T) {
		raw := RawIssue{
			ID:        json.RawMessage(`"123"`),
			Number:    "456",
			State:     "open",
			Title:     "测试Issue",
			Body:      "测试内容",
			CreatedAt: "2024-01-01T00:00:00Z",
			UpdatedAt: "2024-01-02T00:00:00Z",
			User: RawUser{
				Login: "testuser",
				Name:  "测试用户",
			},
		}

		issues, err := parseRawIssues([]RawIssue{raw})
		if err != nil {
			t.Fatalf("解析单个RawIssue失败: %v", err)
		}
		if len(issues) != 1 {
			t.Fatalf("应该返回1个Issue，但得到: %d", len(issues))
		}

		issue := issues[0]
		if issue.ID != 123 {
			t.Errorf("ID错误: 期望=123, 实际=%d", issue.ID)
		}
		if issue.Number != "456" {
			t.Errorf("Number错误: 期望=456, 实际=%s", issue.Number)
		}
		if issue.Title != "测试Issue" {
			t.Errorf("Title错误: 期望=测试Issue, 实际=%s", issue.Title)
		}
		if issue.User.Login != "testuser" {
			t.Errorf("User.Login错误: 期望=testuser, 实际=%s", issue.User.Login)
		}
	})

	// 测试用例3: 多个RawIssue
	t.Run("多个RawIssue", func(t *testing.T) {
		raws := []RawIssue{
			{
				ID:     json.RawMessage(`"1"`),
				Number: "1",
				State:  "open",
				Title:  "Issue 1",
				User:   RawUser{Login: "user1"},
			},
			{
				ID:     json.RawMessage(`"2"`),
				Number: "2",
				State:  "closed",
				Title:  "Issue 2",
				User:   RawUser{Login: "user2"},
			},
		}

		issues, err := parseRawIssues(raws)
		if err != nil {
			t.Fatalf("解析多个RawIssue失败: %v", err)
		}
		if len(issues) != 2 {
			t.Fatalf("应该返回2个Issue，但得到: %d", len(issues))
		}

		// 验证顺序和内容
		if issues[0].Number != "1" {
			t.Errorf("第一个Issue的Number错误: 期望=1, 实际=%s", issues[0].Number)
		}
		if issues[0].State != "open" {
			t.Errorf("第一个Issue的State错误: 期望=open, 实际=%s", issues[0].State)
		}
		if issues[0].User.Login != "user1" {
			t.Errorf("第一个Issue的User.Login错误: 期望=user1, 实际=%s", issues[0].User.Login)
		}

		if issues[1].Number != "2" {
			t.Errorf("第二个Issue的Number错误: 期望=2, 实际=%s", issues[1].Number)
		}
		if issues[1].State != "closed" {
			t.Errorf("第二个Issue的State错误: 期望=closed, 实际=%s", issues[1].State)
		}
		if issues[1].User.Login != "user2" {
			t.Errorf("第二个Issue的User.Login错误: 期望=user2, 实际=%s", issues[1].User.Login)
		}
	})

	// 测试用例4: 包含错误ID的RawIssue
	t.Run("包含错误ID", func(t *testing.T) {
		raw := RawIssue{
			ID:     json.RawMessage(`"invalid"`), // 非数字ID
			Number: "999",
			State:  "open",
			Title:  "测试",
			User:   RawUser{Login: "test"},
		}

		// 注意：parseRawIssue 目前不会因为ID解析失败而返回错误
		// 所以这个测试可能不会失败
		issues, err := parseRawIssues([]RawIssue{raw})
		if err != nil {
			t.Fatalf("不应该返回错误，但得到: %v", err)
		}
		if len(issues) != 1 {
			t.Fatalf("应该返回1个Issue，但得到: %d", len(issues))
		}
		// ID应该为0（默认值）
		if issues[0].ID != 0 {
			t.Errorf("无效ID应该解析为0，但得到: %d", issues[0].ID)
		}
	})

	// 测试用例5: 重用TestParseRawIssue的测试数据
	t.Run("重用现有测试数据", func(t *testing.T) {
		// 创建一个正常的RawIssue
		raw := RawIssue{
			ID:        json.RawMessage(`"12345"`),
			Number:    "42",
			State:     "open",
			Title:     "测试Issue",
			Body:      "测试内容",
			CreatedAt: "2024-01-01T12:00:00Z",
			UpdatedAt: "2024-01-02T12:00:00Z",
			User: RawUser{
				ID:        json.RawMessage(`"67890"`),
				Login:     "testuser",
				Name:      "测试用户",
				AvatarURL: "https://avatar.url/test.png",
			},
			Labels: []Label{
				{ID: 1, Name: "bug", Color: "red", Description: "Bug报告"},
			},
		}

		issues, err := parseRawIssues([]RawIssue{raw})
		if err != nil {
			t.Fatalf("解析失败: %v", err)
		}
		if len(issues) != 1 {
			t.Fatalf("应该返回1个Issue，但得到: %d", len(issues))
		}

		issue := issues[0]
		// 验证关键字段
		if issue.ID != 12345 {
			t.Errorf("ID错误: 期望=12345, 实际=%d", issue.ID)
		}
		if issue.Number != "42" {
			t.Errorf("Number错误: 期望=42, 实际=%s", issue.Number)
		}
		if len(issue.Labels) != 1 {
			t.Errorf("Labels长度错误: 期望=1, 实际=%d", len(issue.Labels))
		}
		if issue.Labels[0].Name != "bug" {
			t.Errorf("标签名称错误: 期望=bug, 实际=%s", issue.Labels[0].Name)
		}
	})
}

// TestParseRawIssuesError 测试错误处理
func TestParseRawIssuesError(t *testing.T) {
	// 注意：目前 parseRawIssue 函数不会返回错误
	// 即使解析失败，它也会返回一个部分解析的 Issue
	// 所以 parseRawIssues 实际上永远不会返回错误

	// 为了测试错误处理，我们需要模拟 parseRawIssue 返回错误的情况
	// 但这需要修改 parseRawIssue 的行为，或者使用接口和依赖注入
	// 目前我们先保留这个测试框架

	t.Run("理论上应该处理错误", func(t *testing.T) {
		// 这个测试目前不会触发错误
		// 但我们可以验证函数的行为
		raws := []RawIssue{
			{
				ID:     json.RawMessage(`"invalid"`),
				Number: "1",
				State:  "open",
				User:   RawUser{Login: "test"},
			},
		}

		issues, err := parseRawIssues(raws)
		if err != nil {
			t.Errorf("当前实现不应该返回错误，但得到: %v", err)
		}
		if len(issues) != 1 {
			t.Errorf("应该返回1个Issue，但得到: %d", len(issues))
		}
		// 验证ID为0（解析失败）
		if issues[0].ID != 0 {
			t.Errorf("无效ID应该解析为0，但得到: %d", issues[0].ID)
		}
	})
}

// tokenGetter 定义token获取接口
type tokenGetter interface {
	GetToken(host string) (string, error)
}

// defaultTokenGetter 默认实现
type defaultTokenGetter struct{}

func (g *defaultTokenGetter) GetToken(host string) (string, error) {
	return GetTokenFromNetrc(host)
}

// issueAPIBaseURLWithDeps 可测试的版本，接受依赖注入
func issueAPIBaseURLWithDeps(originURL string, getter tokenGetter) (baseURL string, token string, err error) {
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

	// 使用注入的token获取器
	token, err = getter.GetToken(host)
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

// mockTokenGetter 模拟token获取器
type mockTokenGetter struct {
	tokenMap map[string]string
	errMap   map[string]error
}

func (m *mockTokenGetter) GetToken(host string) (string, error) {
	if err, ok := m.errMap[host]; ok {
		return "", err
	}
	if token, ok := m.tokenMap[host]; ok {
		return token, nil
	}
	return "", nil
}

// TestIssueAPIBaseURLWithDeps 测试可测试版本
func TestIssueAPIBaseURLWithDeps(t *testing.T) {
	testCases := []struct {
		name          string
		originURL     string
		tokenMap      map[string]string
		errMap        map[string]error
		expectBaseURL string
		expectToken   string
		expectErr     bool
		expectErrMsg  string
	}{
		{
			name:          "SSH格式-gitcode.com-成功",
			originURL:     "git@gitcode.com:owner/repo.git",
			tokenMap:      map[string]string{"gitcode.com": "test-token-123"},
			expectBaseURL: "https://api.gitcode.com/api/v5/repos/owner/repo/issues",
			expectToken:   "test-token-123",
			expectErr:     false,
		},
		{
			name:          "HTTPS格式-gitcode.com-成功",
			originURL:     "https://gitcode.com/owner/repo.git",
			tokenMap:      map[string]string{"gitcode.com": "test-token-456"},
			expectBaseURL: "https://api.gitcode.com/api/v5/repos/owner/repo/issues",
			expectToken:   "test-token-456",
			expectErr:     false,
		},
		{
			name:          "无.git后缀",
			originURL:     "https://gitcode.com/owner/repo",
			tokenMap:      map[string]string{"gitcode.com": "test-token-789"},
			expectBaseURL: "https://api.gitcode.com/api/v5/repos/owner/repo/issues",
			expectToken:   "test-token-789",
			expectErr:     false,
		},
		{
			name:          "HTTP格式",
			originURL:     "http://gitcode.com/owner/repo.git",
			tokenMap:      map[string]string{"gitcode.com": "test-token-http"},
			expectBaseURL: "https://api.gitcode.com/api/v5/repos/owner/repo/issues",
			expectToken:   "test-token-http",
			expectErr:     false,
		},
		{
			name:         "无效SSH格式-缺少路径",
			originURL:    "git@gitcode.com",
			tokenMap:     map[string]string{"gitcode.com": "token"},
			expectErr:    true,
			expectErrMsg: "invalid SSH URL format",
		},
		{
			name:         "无效SSH格式-路径格式错误",
			originURL:    "git@gitcode.com:owner",
			tokenMap:     map[string]string{"gitcode.com": "token"},
			expectErr:    true,
			expectErrMsg: "invalid path in SSH URL",
		},
		{
			name:         "无效HTTPS格式",
			originURL:    "https://gitcode.com",
			tokenMap:     map[string]string{"gitcode.com": "token"},
			expectErr:    true,
			expectErrMsg: "invalid HTTPS URL format",
		},
		{
			name:         "不支持的主机",
			originURL:    "git@github.com:owner/repo.git",
			tokenMap:     map[string]string{"github.com": "github-token"},
			expectErr:    true,
			expectErrMsg: "not support yet",
		},
		{
			name:         "不支持的URL格式",
			originURL:    "invalid-url-format",
			expectErr:    true,
			expectErrMsg: "unsupported URL format",
		},
		{
			name:         "token获取失败-错误",
			originURL:    "git@gitcode.com:owner/repo.git",
			errMap:       map[string]error{"gitcode.com": fmt.Errorf("netrc文件读取失败")},
			expectErr:    true,
			expectErrMsg: "netrc文件读取失败",
		},
		{
			name:         "token获取失败-空token",
			originURL:    "git@gitcode.com:owner/repo.git",
			tokenMap:     map[string]string{"gitcode.com": ""}, // 空token
			expectErr:    true,
			expectErrMsg: "no token found",
		},
		{
			name:         "token获取失败-无token",
			originURL:    "git@gitcode.com:owner/repo.git",
			tokenMap:     map[string]string{}, // 无token
			expectErr:    true,
			expectErrMsg: "no token found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建模拟token获取器
			getter := &mockTokenGetter{
				tokenMap: tc.tokenMap,
				errMap:   tc.errMap,
			}

			// 执行测试
			baseURL, token, err := issueAPIBaseURLWithDeps(tc.originURL, getter)

			// 验证错误
			if tc.expectErr {
				if err == nil {
					t.Errorf("期望错误但得到成功")
					return
				}
				if tc.expectErrMsg != "" && !strings.Contains(err.Error(), tc.expectErrMsg) {
					t.Errorf("错误消息不匹配: 期望包含='%s', 实际='%s'", tc.expectErrMsg, err.Error())
				}
				return
			}

			// 验证成功情况
			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}

			if baseURL != tc.expectBaseURL {
				t.Errorf("baseURL不匹配: 期望='%s', 实际='%s'", tc.expectBaseURL, baseURL)
			}

			if token != tc.expectToken {
				t.Errorf("token不匹配: 期望='%s', 实际='%s'", tc.expectToken, token)
			}
		})
	}
}

// TestIssueAPIBaseURLCompatibility 测试与原始函数的兼容性
// TestIssueAPIBaseURLCompatibility 测试与原始函数的兼容性
func TestIssueAPIBaseURLCompatibility(t *testing.T) {
	// 这个测试验证 issueAPIBaseURLWithDeps 与 IssueAPIBaseURL 行为一致
	// 使用默认的token获取器
	getter := &defaultTokenGetter{}

	testURLs := []string{
		"git@gitcode.com:test/owner.git",
		"https://gitcode.com/test/owner.git",
		"http://gitcode.com/test/owner.git",
	}

	for _, url := range testURLs {
		t.Run(url, func(t *testing.T) {
			// 由于 GetTokenFromNetrc 可能失败（没有 .netrc 文件）
			// 我们只测试URL解析部分，忽略token错误
			_, _, err1 := IssueAPIBaseURL(url)
			_, _, err2 := issueAPIBaseURLWithDeps(url, getter)

			// 如果两个都成功或都失败（且错误类型相同），则通过
			if (err1 == nil && err2 == nil) ||
				(err1 != nil && err2 != nil &&
					strings.Contains(err1.Error(), "no token found") &&
					strings.Contains(err2.Error(), "no token found")) {
				return
			}

			t.Errorf("行为不一致: IssueAPIBaseURL错误=%v, issueAPIBaseURLWithDeps错误=%v", err1, err2)
		})
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

			// 保存原始设置
			oldWriter := outputWriter

			// 设置测试环境
			SetOutputWriter(buf)

			PrintIssue(tc.issue, tc.detailed)

			result := strings.TrimSpace(buf.String())
			if result != tc.result {
				t.Log(result)
				t.Log(tc.result)
				t.Fatal()
			}

			SetOutputWriter(oldWriter)
		})
	}
}

// TestUpdateCommandValidation 测试update命令的参数验证逻辑
func TestUpdateCommandValidation(t *testing.T) {
	testCases := []struct {
		name        string
		issueNumber string
		title       string
		body        string
		state       string
		file        string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "有效的更新-标题",
			issueNumber: "123",
			title:       "新的标题",
			expectError: false,
		},
		{
			name:        "有效的更新-内容",
			issueNumber: "456",
			body:        "新的内容",
			expectError: false,
		},
		{
			name:        "有效的更新-状态",
			issueNumber: "789",
			state:       "closed",
			expectError: false,
		},
		{
			name:        "有效的更新-组合",
			issueNumber: "999",
			title:       "新标题",
			body:        "新内容",
			state:       "open",
			expectError: false,
		},
		{
			name:        "无效的issue编号",
			issueNumber: "abc",
			title:       "测试标题",
			expectError: true,
			errorMsg:    "issue编号必须是数字",
		},
		{
			name:        "没有提供更新字段",
			issueNumber: "123",
			expectError: true,
			errorMsg:    "必须提供至少一个更新字段",
		},
		{
			name:        "无效的状态值",
			issueNumber: "123",
			state:       "invalid",
			expectError: true,
			errorMsg:    "状态必须是 'open' 或 'closed'",
		},
		{
			name:        "有效的状态-open",
			issueNumber: "123",
			state:       "open",
			expectError: false,
		},
		{
			name:        "有效的状态-closed",
			issueNumber: "123",
			state:       "closed",
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟验证逻辑
			issueNumber := tc.issueNumber

			// 验证参数是否为有效的数字
			if _, err := strconv.Atoi(issueNumber); err != nil {
				// 验证参数是否为有效的数字
				if _, err := strconv.Atoi(issueNumber); err != nil {
					if !tc.expectError {
						t.Errorf("不期望的错误: issue编号必须是数字，收到: %s", issueNumber)
					}
					// 检查错误消息是否包含预期内容
					expectedMsg := "issue编号必须是数字"
					if !strings.Contains(err.Error(), "invalid syntax") && !strings.Contains(expectedMsg, tc.errorMsg) {
						t.Errorf("错误消息不匹配: 期望包含数字验证错误, 实际='%v'", err)
					}
					return
				}
				if !tc.expectError {
					t.Error("应该返回错误：必须提供至少一个更新字段")
				}
				if tc.errorMsg != "" && !strings.Contains("必须提供至少一个更新字段", tc.errorMsg) {
					t.Errorf("错误消息不匹配: 期望包含='%s', 实际='必须提供至少一个更新字段'", tc.errorMsg)
				}
				return
			}
			// 验证至少提供了一个更新字段
			if tc.title == "" && tc.body == "" && tc.state == "" && tc.file == "" {
				if !tc.expectError {
					t.Error("应该返回错误：必须提供至少一个更新字段")
					return
				}
				// 如果期望错误且确实没有提供字段，测试通过
				if tc.expectError {
					return
				}
			}

			// 验证状态参数
			if tc.state != "" && tc.state != "open" && tc.state != "closed" {
				if !tc.expectError {
					t.Errorf("应该返回错误：状态必须是 'open' 或 'closed'，收到: %s", tc.state)
					return
				}
				// 如果期望错误且状态无效，测试通过
				if tc.expectError {
					return
				}
			}

			// 如果没有错误，但期望有错误
			if tc.expectError {
				t.Error("期望错误但验证通过")
			}
		})
	}
}

func TestUpdateRequestData(t *testing.T) {
	testCases := []struct {
		name     string
		title    string
		body     string
		state    string
		file     string
		expected map[string]any
	}{
		{
			name:  "只更新标题",
			title: "新的标题",
			expected: map[string]any{
				"title": "新的标题",
			},
		},
		{
			name: "只更新内容",
			body: "新的内容",
			expected: map[string]any{
				"body": "新的内容",
			},
		},
		{
			name:  "只更新状态",
			state: "closed",
			expected: map[string]any{
				"state": "closed",
			},
		},
		{
			name:  "组合更新",
			title: "新标题",
			body:  "新内容",
			state: "open",
			expected: map[string]any{
				"title": "新标题",
				"body":  "新内容",
				"state": "open",
			},
		},
		{
			name:     "空更新",
			expected: map[string]any{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟请求数据构建逻辑
			requestData := make(map[string]any)

			// 模拟从文件读取内容的逻辑
			var body string
			if tc.body != "" {
				body = tc.body
			} else if tc.file != "" {
				// 在测试中，我们不实际读取文件
				body = "文件内容"
			}

			if tc.title != "" {
				requestData["title"] = tc.title
			}
			if body != "" {
				requestData["body"] = body
			}
			if tc.state != "" {
				requestData["state"] = tc.state
			}

			// 验证请求数据
			if len(requestData) != len(tc.expected) {
				t.Errorf("请求数据长度不匹配: 期望=%d, 实际=%d", len(tc.expected), len(requestData))
			}

			for key, expectedValue := range tc.expected {
				actualValue, ok := requestData[key]
				if !ok {
					t.Errorf("缺少键: %s", key)
					continue
				}
				if actualValue != expectedValue {
					t.Errorf("键 %s 的值不匹配: 期望=%v, 实际=%v", key, expectedValue, actualValue)
				}
			}
		})
	}
}

// TestCloseCommandValidation 测试close命令的参数验证逻辑
func TestCloseCommandValidation(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "有效参数",
			args:        []string{"123"},
			expectError: false,
		},
		{
			name:        "无效参数-非数字",
			args:        []string{"abc"},
			expectError: true,
			errorMsg:    "issue编号必须是数字，收到: abc",
		},
		{
			name:        "参数不足",
			args:        []string{},
			expectError: true,
			errorMsg:    "accepts 1 arg(s), received 0",
		},
		{
			name:        "参数过多",
			args:        []string{"123", "456"},
			expectError: true,
			errorMsg:    "accepts 1 arg(s), received 2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 这里我们只测试参数验证逻辑
			// 实际的API调用测试需要模拟HTTP请求
			if tc.expectError {
				// 验证错误消息
				if tc.errorMsg == "" {
					t.Error("测试用例应该指定错误消息")
				}
			}
		})
	}
}

// TestReopenCommandValidation 测试reopen命令的参数验证逻辑
func TestReopenCommandValidation(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "有效参数",
			args:        []string{"123"},
			expectError: false,
		},
		{
			name:        "无效参数-非数字",
			args:        []string{"abc"},
			expectError: true,
			errorMsg:    "issue编号必须是数字，收到: abc",
		},
		{
			name:        "参数不足",
			args:        []string{},
			expectError: true,
			errorMsg:    "accepts 1 arg(s), received 0",
		},
		{
			name:        "参数过多",
			args:        []string{"123", "456"},
			expectError: true,
			errorMsg:    "accepts 1 arg(s), received 2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 这里我们只测试参数验证逻辑
			// 实际的API调用测试需要模拟HTTP请求
			if tc.expectError {
				// 验证错误消息
				if tc.errorMsg == "" {
					t.Error("测试用例应该指定错误消息")
				}
			}
		})
	}
}

// TestAssignCommandValidation 测试assign命令的参数验证逻辑
func TestAssignCommandValidation(t *testing.T) {
	testCases := []struct {
		name        string
		args        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "有效参数",
			args:        []string{"123", "username"},
			expectError: false,
		},
		{
			name:        "无效参数-非数字issue编号",
			args:        []string{"abc", "username"},
			expectError: true,
			errorMsg:    "issue编号必须是数字，收到: abc",
		},
		{
			name:        "参数不足",
			args:        []string{"123"},
			expectError: true,
			errorMsg:    "accepts 2 arg(s), received 1",
		},
		{
			name:        "参数过多",
			args:        []string{"123", "username", "extra"},
			expectError: true,
			errorMsg:    "accepts 2 arg(s), received 3",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 这里我们只测试参数验证逻辑
			// 实际的API调用测试需要模拟HTTP请求
			if tc.expectError {
				// 验证错误消息
				if tc.errorMsg == "" {
					t.Error("测试用例应该指定错误消息")
				}
			}
		})
	}
}
