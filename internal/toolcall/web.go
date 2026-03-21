package toolcall

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html/charset"

	"jaytaylor.com/html2text"
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
func htmlToMarkdown(inputHTML string) (text string) {
	text, err := html2text.FromString(inputHTML, html2text.Options{PrettyTables: true, TextOnly: true})
	if err != nil {
		text = ""
	}
	return
}

// extractMainContent 尝试提取网页主要内容
func extractMainContent(doc *goquery.Document) string {
	removeSelectors := []string{
		"header",
		"footer",
		".ad",
		".quiz-module",
		".subscribe-box",
		"img",
		"[style*='display:none']",
		`[style*="display:none;"]`,
		"[hidden]",
		"img",
		"script", // 删除所有脚本标签
		"style",  // 删除所有样式标签
	}

	for _, sel := range removeSelectors {
		doc.Find(sel).Remove()
	}
	ret, err := doc.Html()
	if err != nil {
		ret = ""
	}
	return ret
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

	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return "", fmt.Errorf("创建编码转换reader失败: %w", err)
	}
	// 获取内容类型
	contentType := resp.Header.Get("Content-Type")
	contentLength := 0

	// 处理不同类型的内容
	var content string
	var isMarkdown bool

	if strings.Contains(contentType, "text/html") {
		doc, err := goquery.NewDocumentFromReader(utf8Reader)
		if err != nil {
			return "", err
		}
		contentLength = doc.Size()
		// 提取主要内容
		mainContent := extractMainContent(doc)
		// 转换为Markdown
		markdownContent := htmlToMarkdown(mainContent)
		// 如果内容太长，截取前一部分
		if len(markdownContent) > 8000 {
			markdownContent = markdownContent[:8000] + "\n\n...（内容已截断，只显示前8000字符）"
		}

		content = markdownContent
		isMarkdown = true

	} else {
		// 如果是JSON，格式化输出
		body, err := io.ReadAll(utf8Reader)
		if err != nil {
			return "", err
		}

		if strings.Contains(contentType, "application/json") {
			var jsonData any
			rawContent := string(body)
			contentLength = len(rawContent)
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

	outfmt.Notice("读取网页: %q", url)
	return result, nil
}

func init() {
	// 注册网页读取工具
	RegisterTool(ToolDef{
		Name:        "web_reader",
		Description: "从互联网获取网页内容并智能转换为Markdown格式。支持HTTP/HTTPS URL，特别适合技术文档阅读和整理。",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "网页URL，如 https://www.baidu.com/s?wd=Golang+教程 或 https://github.com/golang/go",
					"pattern":     TitleLikePattern(1024),
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
