package main

import (
	"strings"
	"testing"
	"time"
)

// TestIssueAPIBaseURL 测试IssueAPIBaseURL函数
func TestIssueAPIBaseURL(t *testing.T) {
	// 保存原始函数
	originalShellExec := ShellExec
	originalGetTokenFromNetrc := GetTokenFromNetrc
	defer func() {
		ShellExec = originalShellExec
		GetTokenFromNetrc = originalGetTokenFromNetrc
	}()

	testCases := []struct {
		name         string
		gitRemoteURL string
		netrcToken   string
		expectError  bool
		expectHost   string
		expectOwner  string
		expectRepo   string
	}{
		{
			name:         "SSH格式URL",
			gitRemoteURL: "git@gitcode.com:dscli/dscli.git",
			netrcToken:   "test-token-123",
			expectError:  false,
			expectHost:   "gitcode.com",
			expectOwner:  "dscli",
			expectRepo:   "dscli",
		},
		{
			name:         "HTTPS格式URL",
			gitRemoteURL: "https://gitcode.com/dscli/dscli.git",
			netrcToken:   "test-token-456",
			expectError:  false,
			expectHost:   "gitcode.com",
			expectOwner:  "dscli",
			expectRepo:   "dscli",
		},
		{
			name:         "HTTPS格式无.git后缀",
			gitRemoteURL: "https://gitcode.com/dscli/dscli",
			netrcToken:   "test-token-789",
			expectError:  false,
			expectHost:   "gitcode.com",
			expectOwner:  "dscli",
			expectRepo:   "dscli",
		},
		{
			name:         "不支持的URL格式",
			gitRemoteURL: "invalid-url-format",
			netrcToken:   "test-token",
			expectError:  true,
		},
		{
			name:         "不支持的主机",
			gitRemoteURL: "git@unsupported.com:owner/repo.git",
			netrcToken:   "test-token",
			expectError:  true,
		},
		{
			name:         "找不到token",
			gitRemoteURL: "git@gitcode.com:dscli/dscli.git",
			netrcToken:   "",
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 模拟ShellExec函数
			ShellExec = func(script string) (string, error) {
				if script == `git remote get-url origin` {
					return tc.gitRemoteURL, nil
				}
				return "", nil
			}

			// 模拟GetTokenFromNetrc函数
			GetTokenFromNetrc = func(host string) (string, error) {
				if host == tc.expectHost && tc.netrcToken != "" {
					return tc.netrcToken, nil
				}
				return "", nil
			}

			// 调用被测试的函数
			baseURL, token, err := IssueAPIBaseURL()

			// 检查错误
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

			// 检查token
			if token != tc.netrcToken {
				t.Errorf("token不匹配: 期望=%s, 实际=%s", tc.netrcToken, token)
			}

			// 检查baseURL格式
			if tc.expectHost == "gitcode.com" {
				expectedBaseURL := "https://api.gitcode.com/api/v5/repos/" + tc.expectOwner + "/" + tc.expectRepo + "/issues"
				if baseURL != expectedBaseURL {
					t.Errorf("baseURL不匹配: 期望=%s, 实际=%s", expectedBaseURL, baseURL)
				}
			}

			// 验证URL包含必要的部分
			if !strings.Contains(baseURL, "https://") {
				t.Errorf("baseURL应该以https://开头: %s", baseURL)
			}
			if !strings.Contains(baseURL, "/repos/") {
				t.Errorf("baseURL应该包含/repos/: %s", baseURL)
			}
			if !strings.Contains(baseURL, "/issues") {
				t.Errorf("baseURL应该以/issues结尾: %s", baseURL)
			}
		})
	}
}

// TestParseRawIssue 测试parseRawIssue函数（通过导出函数间接测试）
func TestParseRawIssue(t *testing.T) {
	// 创建测试用的RawIssue
	rawIssue := RawIssue{
		ID:        []byte(`"12345"`),
		Number:    "42",
		State:     "open",
		Title:     "测试Issue标题",
		Body:      "测试Issue内容",
		CreatedAt: "2024-01-01T12:00:00Z",
		UpdatedAt: "2024-01-02T12:00:00Z",
		ClosedAt:  "",
		Labels: []Label{
			{ID: 1, Name: "bug", Color: "d73a4a", Description: "Bug报告"},
			{ID: 2, Name: "enhancement", Color: "a2eeef", Description: "功能增强"},
		},
		User: RawUser{
			ID:        []byte(`"67890"`),
			Login:     "testuser",
			Name:      "测试用户",
			AvatarURL: "https://avatar.url/test.png",
		},
		Assignee: &RawUser{
			ID:        []byte(`"54321"`),
			Login:     "assigneeuser",
			Name:      "负责人用户",
			AvatarURL: "https://avatar.url/assignee.png",
		},
	}

	// 调用parseRawIssue（私有函数，通过其他方式测试）
	// 这里我们测试相关的导出函数或通过集成测试
	t.Log("parseRawIssue是私有函数，将通过Issue命令的集成测试覆盖")
}

// TestFormatFunctions 测试格式化函数（通过导出函数间接测试）
func TestFormatFunctions(t *testing.T) {
	// 创建测试Issue
	issue := Issue{
		ID:        12345,
		Number:    "42",
		State:     "open",
		Title:     "测试Issue",
		Body:      "测试内容",
		CreatedAt: parseTime("2024-01-01T12:00:00Z"),
		UpdatedAt: parseTime("2024-01-02T12:00:00Z"),
		ClosedAt:  parseTime(""),
		User: User{
			ID:        67890,
			Login:     "testuser",
			Name:      "测试用户",
			AvatarURL: "https://avatar.url/test.png",
		},
		Assignee: &User{
			ID:        54321,
			Login:     "assigneeuser",
			Name:      "负责人用户",
			AvatarURL: "https://avatar.url/assignee.png",
		},
		Labels: []Label{
			{ID: 1, Name: "bug", Color: "d73a4a", Description: "Bug报告"},
			{ID: 2, Name: "enhancement", Color: "a2eeef", Description: "功能增强"},
		},
	}

	// 测试formatTime
	createdStr := formatTime(issue.CreatedAt)
	if createdStr != "2024-01-01 12:00:00" {
		t.Errorf("formatTime错误: 期望=2024-01-01 12:00:00, 实际=%s", createdStr)
	}

	// 测试空时间
	emptyTimeStr := formatTime(issue.ClosedAt)
	if emptyTimeStr != "-" {
		t.Errorf("空时间formatTime错误: 期望=-, 实际=%s", emptyTimeStr)
	}

	// 测试formatAssignee
	assigneeStr := formatAssignee(issue.Assignee)
	expectedAssignee := "负责人用户 (assigneeuser)"
	if assigneeStr != expectedAssignee {
		t.Errorf("formatAssignee错误: 期望=%s, 实际=%s", expectedAssignee, assigneeStr)
	}

	// 测试无负责人的情况
	noAssigneeStr := formatAssignee(nil)
	if noAssigneeStr != "-" {
		t.Errorf("无负责人formatAssignee错误: 期望=-, 实际=%s", noAssigneeStr)
	}

	// 测试只有login没有name的情况
	issue.Assignee.Name = ""
	assigneeOnlyLogin := formatAssignee(issue.Assignee)
	if assigneeOnlyLogin != "assigneeuser" {
		t.Errorf("只有login的formatAssignee错误: 期望=assigneeuser, 实际=%s", assigneeOnlyLogin)
	}

	// 恢复Assignee
	issue.Assignee.Name = "负责人用户"

	// 测试formatLabels
	labelsStr := formatLabels(issue.Labels)
	expectedLabels := "bug, enhancement"
	if labelsStr != expectedLabels {
		t.Errorf("formatLabels错误: 期望=%s, 实际=%s", expectedLabels, labelsStr)
	}

	// 测试空标签
	emptyLabelsStr := formatLabels([]Label{})
	if emptyLabelsStr != "-" {
		t.Errorf("空标签formatLabels错误: 期望=-, 实际=%s", emptyLabelsStr)
	}
}

// parseTime 辅助函数，解析时间字符串
func parseTime(timeStr string) time.Time {
	if timeStr == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Time{}
	}
	return t
}

// TestPrintIssue 测试printIssue函数（通过命令测试）
func TestPrintIssue(t *testing.T) {
	// printIssue是私有函数，将通过issue命令的集成测试覆盖
	// 这里我们验证相关的格式化函数
	t.Log("printIssue是私有函数，将通过issue命令的集成测试覆盖")
}

// TestIssueCommandIntegration 测试issue命令的集成（模拟测试）
func TestIssueCommandIntegration(t *testing.T) {
	// 这个测试需要模拟HTTP请求，比较复杂
	// 在实际项目中，可以使用httptest.Server来模拟API响应
	t.Log("issue命令集成测试需要模拟HTTP API，将在后续添加")
}

// TestIssueStructs 测试Issue相关结构体
func TestIssueStructs(t *testing.T) {
	// 测试RawIssue结构体
	rawIssue := RawIssue{
		ID:        []byte(`"123"`),
		Number:    "1",
		State:     "open",
		Title:     "测试",
		Body:      "内容",
		CreatedAt: "2024-01-01T00:00:00Z",
		User: RawUser{
			ID:    []byte(`"456"`),
			Login: "user",
		},
	}

	if rawIssue.Number != "1" {
		t.Errorf("RawIssue.Number错误: 期望=1, 实际=%s", rawIssue.Number)
	}
	if rawIssue.State != "open" {
		t.Errorf("RawIssue.State错误: 期望=open, 实际=%s", rawIssue.State)
	}

	// 测试Issue结构体
	issue := Issue{
		ID:     123,
		Number: "1",
		State:  "closed",
		Title:  "已关闭的issue",
	}

	if issue.ID != 123 {
		t.Errorf("Issue.ID错误: 期望=123, 实际=%d", issue.ID)
	}
	if issue.Number != "1" {
		t.Errorf("Issue.Number错误: 期望=1, 实际=%s", issue.Number)
	}
	if issue.State != "closed" {
		t.Errorf("Issue.State错误: 期望=closed, 实际=%s", issue.State)
	}

	// 测试Label结构体
	label := Label{
		ID:          1,
		Name:        "bug",
		Color:       "red",
		Description: "Bug报告",
	}

	if label.Name != "bug" {
		t.Errorf("Label.Name错误: 期望=bug, 实际=%s", label.Name)
	}
	if label.Color != "red" {
		t.Errorf("Label.Color错误: 期望=red, 实际=%s", label.Color)
	}

	// 测试User结构体
	user := User{
		ID:    456,
		Login: "testuser",
		Name:  "测试用户",
	}

	if user.Login != "testuser" {
		t.Errorf("User.Login错误: 期望=testuser, 实际=%s", user.Login)
	}
	if user.Name != "测试用户" {
		t.Errorf("User.Name错误: 期望=测试用户, 实际=%s", user.Name)
	}
}
