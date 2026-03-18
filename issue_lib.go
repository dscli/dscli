package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// GetIssueConfig 获取issue配置信息
func GetIssueConfig(ctx context.Context) (issueConfig *IssueConfig, err error) {
	// 使用context.Background()，因为CLI命令可能没有传递context
	// 在实际调用中，ShellExec会处理context
	issueConfig = &IssueConfig{}
	originURL, err := ShellExec(ctx, `git remote get-url origin`)
	if err != nil {
		return nil, fmt.Errorf("获取git远程URL失败: %w", err)
	}

	err = IssueAPIBaseURL(originURL, issueConfig)
	if err != nil {
		return nil, fmt.Errorf("解析API URL失败: %w", err)
	}
	return
}

// ListIssues 列出issues
func ListIssues(ctx context.Context, state string) ([]Issue, error) {
	config, err := GetIssueConfig(ctx)
	if err != nil {
		return nil, err
	}

	// 构建URL
	url := fmt.Sprintf("%s/issues?access_token=%s&state=%s", config.BaseURL, config.Token, state)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求issue列表失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &IssueAPIError{
			StatusCode: resp.StatusCode,
			Message:    "API请求失败",
			Details:    string(body),
		}
	}

	// 读取响应
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析为RawIssue数组
	var rawIssues []RawIssue
	err = json.Unmarshal(b, &rawIssues)
	if err != nil {
		return nil, fmt.Errorf("解析issue列表失败: %w", err)
	}

	// 转换为Issue数组
	issues, err := parseRawIssues(rawIssues)
	if err != nil {
		return nil, fmt.Errorf("处理issue数据失败: %w", err)
	}

	return issues, nil
}

// ShowIssue 显示单个issue详情
func ShowIssue(ctx context.Context, number int) (*Issue, error) {
	if number == 0 {
		return nil, fmt.Errorf("必须提供issue编号")
	}

	config, err := GetIssueConfig(ctx)
	if err != nil {
		return nil, err
	}

	// 构建URL
	url := fmt.Sprintf("%s/issues/%d?access_token=%s", config.BaseURL, number, config.Token)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("请求issue详情失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &IssueAPIError{
			StatusCode: resp.StatusCode,
			Message:    "API请求失败",
			Details:    string(body),
		}
	}

	// 读取响应
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析为RawIssue
	var rawIssue RawIssue
	err = json.Unmarshal(b, &rawIssue)
	if err != nil {
		return nil, fmt.Errorf("解析issue数据失败: %w", err)
	}

	// 转换为Issue
	issue, err := parseRawIssue(rawIssue)
	if err != nil {
		return nil, fmt.Errorf("处理issue数据失败: %w", err)
	}

	return &issue, nil
}

type CreateIssueOptions struct {
	Title string
	Body  string
}

func CreateIssue(ctx context.Context, opts CreateIssueOptions) (*Issue, error) {
	if opts.Title == "" {
		return nil, fmt.Errorf("必须提供issue标题")
	}

	config, err := GetIssueConfig(ctx)
	if err != nil {
		return nil, err
	}

	// 准备请求数据
	requestData := map[string]any{
		"repo":        config.Repo,
		"title":       opts.Title,
		"description": opts.Body,
	}
	jsonData, err := JSONMarshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("序列化请求数据失败: %w", err)
	}

	// 发送POST请求
	url := fmt.Sprintf("%s/issues?access_token=%s", config.BaseURL, config.Token)
	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, &IssueAPIError{
			StatusCode: resp.StatusCode,
			Message:    "创建issue失败",
			Details:    string(body),
		}
	}

	// 读取响应
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析为RawIssue
	var rawIssue RawIssue
	err = json.Unmarshal(b, &rawIssue)
	if err != nil {
		return nil, fmt.Errorf("解析响应数据失败: %w", err)
	}

	// 转换为Issue
	issue, err := parseRawIssue(rawIssue)
	if err != nil {
		return nil, fmt.Errorf("处理issue数据失败: %w", err)
	}

	return &issue, nil
}

type UpdateIssueOptions struct {
	Number int
	Title  string
	Body   string
	State  string
}

func UpdateIssue(ctx context.Context, opts UpdateIssueOptions) (*Issue, error) {
	if opts.Number == 0 {
		return nil, fmt.Errorf("必须提供issue编号")
	}

	// 验证至少提供了一个更新字段
	if opts.Title == "" && opts.Body == "" && opts.State == "" {
		return nil, fmt.Errorf("必须提供至少一个更新字段（Title, Body 或 State）")
	}

	// 验证状态参数
	if opts.State != "" && opts.State != "open" && opts.State != "closed" {
		return nil, fmt.Errorf("状态必须是 'open' 或 'closed'，收到: %s", opts.State)
	}

	config, err := GetIssueConfig(ctx)
	if err != nil {
		return nil, err
	}

	// 准备请求数据
	requestData := make(map[string]any)
	// GitCode API要求包含repo字段
	if config.Repo != "" {
		requestData["repo"] = config.Repo
	}

	if opts.Title != "" {
		requestData["title"] = opts.Title
	}
	if opts.Body != "" {
		requestData["description"] = opts.Body
	}
	if opts.State != "" {
		// GitCode API 使用 "state_event" 而不是 "state"
		switch opts.State {
		case "closed":
			requestData["state_event"] = "close"
		case "open":
			requestData["state_event"] = "reopen"
		}
	}

	// GitCode API 要求至少提供一个参数，当只更新状态时，添加一个有效的参数
	// 根据错误信息，state_event可能不被算作"至少一个参数"，所以我们需要添加另一个参数
	// 尝试使用labels参数，设置为空数组
	if opts.Title == "" && opts.Body == "" && opts.State != "" {
		// 添加labels字段，设置为空数组
		requestData["labels"] = []string{}
	}
	jsonData, err := JSONMarshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("序列化请求数据失败: %w", err)
	}

	// 发送PATCH请求
	// GitCode API格式: PATCH /api/v5/repos/:owner/issues/:number
	// 注意：URL中不包含repo，repo在请求体中
	url := fmt.Sprintf("%s/issues/%d?access_token=%s", config.BaseURL, opts.Number, config.Token)
	req, err := http.NewRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &IssueAPIError{
			StatusCode: resp.StatusCode,
			Message:    "更新issue失败",
			Details:    string(body),
		}
	}

	// 读取响应
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析为RawIssue
	var rawIssue RawIssue
	err = json.Unmarshal(b, &rawIssue)
	if err != nil {
		return nil, fmt.Errorf("解析响应数据失败: %w", err)
	}

	// 转换为Issue
	issue, err := parseRawIssue(rawIssue)
	if err != nil {
		return nil, fmt.Errorf("处理issue数据失败: %w", err)
	}

	return &issue, nil
}

// CloseIssue 关闭issue
func CloseIssue(ctx context.Context, number int) (issue *Issue, err error) {
	// 使用UpdateIssue来关闭issue，这样可以利用已有的逻辑
	issue, err = UpdateIssue(ctx, UpdateIssueOptions{
		Number: number,
		State:  "closed",
	})
	if err != nil {
		return nil, fmt.Errorf("关闭issue失败: %w", err)
	}
	return issue, nil
}

// ReopenIssue 重新打开issue
func ReopenIssue(ctx context.Context, number int) (issue *Issue, err error) {
	// 使用UpdateIssue来重新打开issue
	issue, err = UpdateIssue(ctx, UpdateIssueOptions{
		Number: number,
		State:  "open",
	})
	if err != nil {
		return nil, fmt.Errorf("重新打开issue失败: %w", err)
	}
	return issue, nil
}

// AssignIssue 分配issue给用户
func AssignIssue(ctx context.Context, number int, username string) (*Issue, error) {
	if number == 0 {
		return nil, fmt.Errorf("必须提供issue编号")
	}
	if username == "" {
		return nil, fmt.Errorf("必须提供用户名")
	}

	config, err := GetIssueConfig(ctx)
	if err != nil {
		return nil, err
	}

	// 准备请求数据 - 分配issue
	requestData := map[string]any{
		"repo":         config.Repo,
		"assignee_ids": []string{username},
	}

	// 转换为JSON
	jsonData, err := JSONMarshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("序列化请求数据失败: %w", err)
	}

	// 发送PATCH请求
	url := fmt.Sprintf("%s/issues/%d?access_token=%s", config.BaseURL, number, config.Token)
	req, err := http.NewRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &IssueAPIError{
			StatusCode: resp.StatusCode,
			Message:    "分配issue失败",
			Details:    string(body),
		}
	}

	// 读取响应
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析为RawIssue
	var rawIssue RawIssue
	err = json.Unmarshal(b, &rawIssue)
	if err != nil {
		return nil, fmt.Errorf("解析响应数据失败: %w", err)
	}

	// 转换为Issue
	issue, err := parseRawIssue(rawIssue)
	if err != nil {
		return nil, fmt.Errorf("处理issue数据失败: %w", err)
	}

	return &issue, nil
}

// ReadBodyFromStdinOrFile 从标准输入或文件读取内容
func ReadBodyFromStdinOrFile(filePath string) (string, error) {
	var body string

	if filePath != "" {
		// 从文件读取
		content, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("读取文件失败: %w", err)
		}
		body = string(content)
	} else {
		// 检查是否有标准输入
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// 有标准输入数据
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return "", fmt.Errorf("读取标准输入失败: %w", err)
			}
			body = string(data)
		}
	}

	return body, nil
}
