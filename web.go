package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// 创建HTTP客户端，设置超时
var webClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		DisableKeepAlives: true,
	},
}

// handleWebReader 读取网页内容
func handleWebReader(ctx context.Context, args map[string]string) (string, error) {
	url, ok := args["url"]
	if !ok || url == "" {
		return "", fmt.Errorf("no URL or empty URL specified")
	}

	// 确保URL以http://或https://开头
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return Web2Markdown(ctx, url)
}

// Web2Markdown fetch web page and convert it to markdown
func Web2Markdown(ctx context.Context, url string) (string, error) {
	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置User-Agent，模拟浏览器访问
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Connection", "close")

	// 记录开始时间
	startTime := time.Now()

	// 发送请求
	resp, err := webClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP请求失败，状态码: %d", resp.StatusCode)
	}

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	// 获取内容类型
	contentType := resp.Header.Get("Content-Type")
	contentLength := len(body)

	// 如果是HTML，尝试提取文本内容
	var content string
	if strings.Contains(contentType, "text/html") {
		// 这里可以添加HTML解析逻辑，提取纯文本
		// 目前先返回原始HTML，但限制长度
		content = string(body)
		// 如果内容太长，截取前一部分
		if len(content) > 10000 {
			content = content[:10000] + "\n\n...（内容已截断，只显示前10000字符）"
		}
	} else if strings.Contains(contentType, "application/json") {
		// 如果是JSON，格式化输出
		var jsonData any
		content = string(body)
		body = []byte(strings.TrimSpace(content))
		if len(body) > 0 {
			if body[0] == '[' {
				jsonData = []map[string]any{}
			} else {
				jsonData = map[string]any{}
			}
		}
		if err := json.Unmarshal(body, &jsonData); err == nil {
			formatted, _ := json.MarshalIndent(jsonData, "", "  ")
			content = string(formatted)
		} else {
			content = string(body)
		}
	} else if strings.Contains(contentType, "text/plain") {
		content = string(body)
		// 如果内容太长，截取前一部分
		if len(content) > 10000 {
			content = content[:10000] + "\n\n...（内容已截断，只显示前10000字符）"
		}
	} else {
		content = fmt.Sprintf("二进制内容（Content-Type: %s，大小: %d 字节）", contentType, contentLength)
	}

	// 计算执行时间
	executionTime := time.Since(startTime)

	// 构建结果
	result := fmt.Sprintf(`=== 执行结果 ===
网页内容:
%s

网页信息:
- URL: %s
- 状态码: %d
- 内容类型: %s
- 内容大小: %d 字节
- 响应时间: %v

=== 执行统计 ===
执行时间: %v
状态: 成功`,
		content,
		url,
		resp.StatusCode,
		contentType,
		contentLength,
		executionTime,
		executionTime)

	Notice("读取网页: \"%s\"（%d字节）", url, contentLength)
	return result, nil
}

func init() {
	// 注册网页读取工具
	RegisterTool(ToolDef{
		Name:        "web_reader",
		Description: "读取网页内容，支持HTTP/HTTPS URL",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "网页URL，如 https://example.com",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
		Category: "web",
		Timeout:  60 * time.Second, // 60秒超时
		Handler:  handleWebReader,
	})
}
