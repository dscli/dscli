package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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
func handleWebReader(ctx context.Context, args ToolArgs) (string, error) {
	url := ToolArgsValue(args, "url", "")
	if url == "" {
		return "", fmt.Errorf("no URL or empty URL specified")
	}

	// 确保URL以http://或https://开头
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	return Web2Markdown(ctx, url)
}

// htmlToMarkdown 将HTML转换为简化的Markdown格式
func htmlToMarkdown(html string) string {
	// 移除脚本和样式标签
	html = regexp.MustCompile(`(?is)<script.*?>.*?</script>`).ReplaceAllString(html, "")
	html = regexp.MustCompile(`(?is)<style.*?>.*?</style>`).ReplaceAllString(html, "")

	// 处理标题
	html = regexp.MustCompile(`(?is)<h1.*?>(.*?)</h1>`).ReplaceAllString(html, "# $1\n\n")
	html = regexp.MustCompile(`(?is)<h2.*?>(.*?)</h2>`).ReplaceAllString(html, "## $1\n\n")
	html = regexp.MustCompile(`(?is)<h3.*?>(.*?)</h3>`).ReplaceAllString(html, "### $1\n\n")
	html = regexp.MustCompile(`(?is)<h4.*?>(.*?)</h4>`).ReplaceAllString(html, "#### $1\n\n")
	html = regexp.MustCompile(`(?is)<h5.*?>(.*?)</h5>`).ReplaceAllString(html, "##### $1\n\n")
	html = regexp.MustCompile(`(?is)<h6.*?>(.*?)</h6>`).ReplaceAllString(html, "###### $1\n\n")

	// 处理段落
	html = regexp.MustCompile(`(?is)<p.*?>(.*?)</p>`).ReplaceAllString(html, "$1\n\n")

	// 处理链接 [text](url)
	html = regexp.MustCompile(`(?is)<a.*?href="(.*?)".*?>(.*?)</a>`).ReplaceAllString(html, "[$2]($1)")

	// 处理图片 ![alt](src)
	html = regexp.MustCompile(`(?is)<img.*?src="(.*?)".*?alt="(.*?)".*?>`).ReplaceAllString(html, "![$2]($1)")
	html = regexp.MustCompile(`(?is)<img.*?src="(.*?)".*?>`).ReplaceAllString(html, "![]($1)")

	// 处理列表项
	html = regexp.MustCompile(`(?is)<li.*?>(.*?)</li>`).ReplaceAllString(html, "- $1\n")

	// 处理代码块
	html = regexp.MustCompile(`(?is)<pre.*?>(.*?)</pre>`).ReplaceAllString(html, "```\n$1\n```\n\n")
	html = regexp.MustCompile(`(?is)<code.*?>(.*?)</code>`).ReplaceAllString(html, "`$1`")

	// 处理加粗和斜体
	html = regexp.MustCompile(`(?is)<strong.*?>(.*?)</strong>`).ReplaceAllString(html, "**$1**")
	html = regexp.MustCompile(`(?is)<b.*?>(.*?)</b>`).ReplaceAllString(html, "**$1**")
	html = regexp.MustCompile(`(?is)<em.*?>(.*?)</em>`).ReplaceAllString(html, "*$1*")
	html = regexp.MustCompile(`(?is)<i.*?>(.*?)</i>`).ReplaceAllString(html, "*$1*")

	// 移除所有HTML标签
	html = regexp.MustCompile(`(?is)<.*?>`).ReplaceAllString(html, "")

	// 处理HTML实体
	html = strings.ReplaceAll(html, "&lt;", "<")
	html = strings.ReplaceAll(html, "&gt;", ">")
	html = strings.ReplaceAll(html, "&amp;", "&")
	html = strings.ReplaceAll(html, "&quot;", "\"")
	html = strings.ReplaceAll(html, "&#39;", "'")
	html = strings.ReplaceAll(html, "&nbsp;", " ")

	// 清理多余的空行
	html = regexp.MustCompile(`\n{3,}`).ReplaceAllString(html, "\n\n")
	html = strings.TrimSpace(html)

	return html
}

// extractMainContent 尝试提取网页主要内容
func extractMainContent(html string) string {
	// 简单的启发式方法：找到包含最多文本的div
	// 这里先使用简单的实现，后续可以改进
	return html
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

	// 处理不同类型的内容
	var content string
	var isMarkdown bool

	if strings.Contains(contentType, "text/html") {
		// HTML转Markdown
		htmlContent := string(body)
		// 提取主要内容
		mainContent := extractMainContent(htmlContent)
		// 转换为Markdown
		markdownContent := htmlToMarkdown(mainContent)

		// 如果内容太长，截取前一部分
		if len(markdownContent) > 8000 {
			markdownContent = markdownContent[:8000] + "\n\n...（内容已截断，只显示前8000字符）"
		}

		content = markdownContent
		isMarkdown = true

	} else if strings.Contains(contentType, "application/json") {
		// 如果是JSON，格式化输出
		var jsonData any
		rawContent := string(body)
		trimmedBody := []byte(strings.TrimSpace(rawContent))
		if len(trimmedBody) > 0 {
			if trimmedBody[0] == '[' {
				jsonData = []map[string]any{}
			} else {
				jsonData = map[string]any{}
			}
		}
		if err := json.Unmarshal(trimmedBody, &jsonData); err == nil {
			formatted, _ := json.MarshalIndent(jsonData, "", "  ")
			content = string(formatted)
		} else {
			content = rawContent
		}
		isMarkdown = false

	} else if strings.Contains(contentType, "text/plain") {
		content = string(body)
		// 如果内容太长，截取前一部分
		if len(content) > 10000 {
			content = content[:10000] + "\n\n...（内容已截断，只显示前10000字符）"
		}
		isMarkdown = strings.Contains(contentType, "markdown") || strings.HasSuffix(url, ".md")

	} else {
		content = fmt.Sprintf("二进制内容（Content-Type: %s，大小: %d 字节）", contentType, contentLength)
		isMarkdown = false
	}

	// 计算执行时间
	executionTime := time.Since(startTime)

	// 构建结果
	formatInfo := "原始格式"
	if isMarkdown {
		formatInfo = "Markdown格式"
	}

	result := fmt.Sprintf(`📝 执行结果:
网页内容（%s）:
%s

网页信息:
- URL: %s
- 状态码: %d
- 内容类型: %s
- 内容大小: %d 字节
- 响应时间: %v
- 输出格式: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
		formatInfo,
		content,
		url,
		resp.StatusCode,
		contentType,
		contentLength,
		executionTime,
		formatInfo,
		executionTime)

	Notice("读取网页: \"%s\"（%d字节，转换为%s）", url, contentLength, formatInfo)
	return result, nil
}

func init() {
	// 注册网页读取工具
	RegisterTool(ToolDef{
		Name:        "web_reader",
		Description: "从互联网获取网页内容并智能转换为Markdown格式。支持HTTP/HTTPS URL，特别适合技术文档阅读和整理。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "网页URL，如 https://www.baidu.com/s?wd=Golang+教程 或 https://github.com/golang/go",
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
