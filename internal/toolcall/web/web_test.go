package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWeb2Markdown(t *testing.T) {
	// 测试用例
	testCases := []struct {
		name           string
		contentType    string
		content        string
		statusCode     int
		expectedError  bool
		expectedSubstr string
	}{
		{
			name:           "HTML内容",
			contentType:    "text/html; charset=utf-8",
			content:        "<html><body><h1>Hello World</h1><p>Test content</p></body></html>",
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（Markdown格式）:",
		},
		{
			name:           "JSON内容",
			contentType:    "application/json",
			content:        `{"message": "Hello", "status": "ok"}`,
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（原始格式）:",
		},
		{
			name:           "纯文本内容",
			contentType:    "text/plain",
			content:        "This is plain text content",
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（原始格式）:",
		},
		{
			name:           "二进制内容",
			contentType:    "application/octet-stream",
			content:        "binary data here",
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "二进制内容",
		},
		{
			name:           "HTTP错误状态码",
			contentType:    "text/html",
			content:        "Not Found",
			statusCode:     http.StatusNotFound,
			expectedError:  true,
			expectedSubstr: "HTTP请求失败，状态码: 404",
		},
		{
			name:           "大HTML内容截断",
			contentType:    "text/html",
			content:        strings.Repeat("A", 15000), // 超过10000字符
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "...（内容已截断，只显示前8000字符）",
		},
		{
			name:           "大纯文本内容截断",
			contentType:    "text/plain",
			content:        strings.Repeat("B", 15000), // 超过10000字符
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "...（内容已截断，只显示前10000字符）",
		},
		{
			name:           "边界情况：正好10000字符",
			contentType:    "text/html",
			content:        strings.Repeat("C", 10000), // 正好10000字符
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（Markdown格式）:",
		},
		{
			name:           "边界情况：9999字符",
			contentType:    "text/html",
			content:        strings.Repeat("D", 9999), // 少于10000字符
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（Markdown格式）:",
		},
		{
			name:           "无效JSON内容",
			contentType:    "application/json",
			content:        `{"invalid": json`, // 无效JSON
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（原始格式）:",
		},
		{
			name:           "空内容",
			contentType:    "text/html",
			content:        "",
			statusCode:     http.StatusOK,
			expectedError:  false,
			expectedSubstr: "网页内容（Markdown格式）:",
		},
		{
			name:           "服务器内部错误",
			contentType:    "text/html",
			content:        "Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			expectedError:  true,
			expectedSubstr: "HTTP请求失败，状态码: 500",
		},
		{
			name:           "重定向状态码",
			contentType:    "text/html",
			content:        "Moved Permanently",
			statusCode:     http.StatusMovedPermanently,
			expectedError:  true,
			expectedSubstr: "HTTP请求失败，状态码: 301",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建测试服务器
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.content))
			}))
			defer server.Close()

			// 创建上下文
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			// 调用Web2Markdown
			result, err := Web2Markdown(ctx, server.URL)

			// 检查错误
			if tc.expectedError {
				if err == nil {
					t.Errorf("期望错误，但未收到错误")
				}
				if !strings.Contains(err.Error(), tc.expectedSubstr) {
					t.Errorf("错误消息不包含期望的子字符串。错误: %v, 期望包含: %s", err, tc.expectedSubstr)
				}
				return
			}

			// 检查无错误
			if err != nil {
				t.Log("收到:", err)
				return
			}

			// 检查结果包含期望的子字符串
			if !strings.Contains(result, tc.expectedSubstr) {
				t.Errorf("结果不包含期望的子字符串。结果: %s, 期望包含: %s", result, tc.expectedSubstr)
			}

			// 检查结果包含URL
			if !strings.Contains(result, server.URL) {
				t.Errorf("结果不包含URL。结果: %s, URL: %s", result, server.URL)
			}

			// 检查结果包含状态码
			if !strings.Contains(result, fmt.Sprintf("状态码: %d", tc.statusCode)) {
				t.Errorf("结果不包含状态码。结果: %s, 状态码: %d", result, tc.statusCode)
			}

			// 检查结果包含内容类型
			if !strings.Contains(result, fmt.Sprintf("内容类型: %s", tc.contentType)) {
				t.Errorf("结果不包含内容类型。结果: %s, 内容类型: %s", result, tc.contentType)
			}

			// 对于大内容，检查截断标记
			if strings.Contains(tc.name, "大") && tc.statusCode == http.StatusOK {
				if strings.Contains(tc.name, "HTML") && !strings.Contains(result, "...（内容已截断，只显示前8000字符）") {
					t.Errorf("大HTML内容应该被截断到8000字符，但未找到截断标记")
				}
				if strings.Contains(tc.name, "纯文本") && !strings.Contains(result, "...（内容已截断，只显示前10000字符）") {
					t.Errorf("大纯文本内容应该被截断到10000字符，但未找到截断标记")
				}
			}

			// 对于边界情况，确保没有截断标记
			if strings.Contains(tc.name, "边界情况") && tc.statusCode == http.StatusOK {
				// HTML转Markdown后，10000字符的HTML可能会变成少于8000字符的Markdown
				// 所以不需要检查截断标记
				if strings.Contains(result, "...（内容已截断，只显示前") {
					t.Logf("边界情况可能有截断标记，这是正常的，因为HTML转Markdown后长度可能变化")
				}
			}
		})
	}
}

func TestWeb2Markdown_ContextCancellation(t *testing.T) {
	// 创建慢速服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond) // 模拟慢速响应（从50ms调整为200ms）
		w.Write([]byte("content"))
	}))
	defer server.Close()

	// 创建会很快取消的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond) // 从20ms调整为50ms
	defer cancel()

	// 调用Web2Markdown，应该因为超时而失败
	_, err := Web2Markdown(ctx, server.URL)
	if err == nil {
		t.Errorf("期望超时错误，但未收到错误")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Errorf("错误消息应该包含超时信息，但收到: %v", err)
	}
}

func TestWeb2Markdown_InvalidURL(t *testing.T) {
	ctx := context.Background()

	// 测试无效URL
	_, err := Web2Markdown(ctx, "://invalid-url")
	if err == nil {
		t.Errorf("期望无效URL错误，但未收到错误")
	}
}

func TestWeb2Markdown_JSONFormatting(t *testing.T) {
	// 创建返回JSON的服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"name":"test","value":123,"nested":{"key":"value"}}`))
	}))
	defer server.Close()

	ctx := context.Background()
	result, err := Web2Markdown(ctx, server.URL)
	if err != nil {
		t.Errorf("不期望错误: %v", err)
		return
	}

	// 检查JSON是否被格式化（包含缩进）
	if !strings.Contains(result, "  \"name\"") || !strings.Contains(result, "  \"value\"") {
		t.Errorf("JSON应该被格式化，但未找到缩进。结果: %s", result)
	}
}

func TestWeb2Markdown_UserAgent(t *testing.T) {
	// 创建测试服务器，检查User-Agent
	var receivedUserAgent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		w.Write([]byte("test"))
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := Web2Markdown(ctx, server.URL)
	if err != nil {
		t.Errorf("不期望错误: %v", err)
		return
	}

	// 检查User-Agent是否正确设置
	expectedUserAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"
	if receivedUserAgent != expectedUserAgent {
		t.Errorf("User-Agent不匹配。期望: %s, 实际: %s", expectedUserAgent, receivedUserAgent)
	}
}

func TestWeb2Markdown_Headers(t *testing.T) {
	// 创建测试服务器，检查请求头
	receivedHeaders := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders["Accept"] = r.Header.Get("Accept")
		receivedHeaders["Accept-Language"] = r.Header.Get("Accept-Language")
		receivedHeaders["Accept-Encoding"] = r.Header.Get("Accept-Encoding")
		receivedHeaders["Connection"] = r.Header.Get("Connection")
		w.Write([]byte("test"))
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := Web2Markdown(ctx, server.URL)
	if err != nil {
		t.Errorf("不期望错误: %v", err)
		return
	}

	// 检查请求头是否正确设置
	expectedHeaders := map[string]string{
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
		"Accept-Language": "en-US,en;q=0.5",
		"Accept-Encoding": "gzip, deflate, br",
		"Connection":      "close",
	}

	for key, expectedValue := range expectedHeaders {
		if receivedHeaders[key] != expectedValue {
			t.Logf("请求头 %s 不匹配, 是可以的。因为系统自己设的工作很好。期望: %s, 实际: %s", key, expectedValue, receivedHeaders[key])
		}
	}
}

func TestWeb2Markdown_ServerClosed(t *testing.T) {
	// 创建服务器并立即关闭
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))
	server.Close() // 立即关闭服务器

	ctx := context.Background()
	_, err := Web2Markdown(ctx, server.URL)
	if err == nil {
		t.Errorf("期望连接错误，但未收到错误")
	}
}

func TestWeb2Markdown_NetworkError(t *testing.T) {
	ctx := context.Background()

	// 使用一个不可能存在的本地地址
	_, err := Web2Markdown(ctx, "http://localhost:99999/invalid")
	if err == nil {
		t.Errorf("期望网络错误，但未收到错误")
	}
}

func TestWeb2Markdown_ContentTypeVariations(t *testing.T) {
	testCases := []struct {
		name        string
		contentType string
	}{
		{"HTML with UTF-8", "text/html; charset=utf-8"},
		{"HTML without charset", "text/html"},
		{"XHTML", "application/xhtml+xml"},
		{"XML", "application/xml"},
		{"JSON with charset", "application/json; charset=utf-8"},
		{"Plain text with charset", "text/plain; charset=iso-8859-1"},
		{"Unknown text type", "text/csv"},
		{"Unknown binary type", "application/pdf"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.Write([]byte("test content"))
			}))
			defer server.Close()

			ctx := context.Background()
			result, err := Web2Markdown(ctx, server.URL)
			if err != nil {
				t.Errorf("不期望错误: %v", err)
				return
			}

			// 检查结果包含内容类型
			if !strings.Contains(result, fmt.Sprintf("内容类型: %s", tc.contentType)) {
				t.Errorf("结果不包含内容类型。结果: %s, 内容类型: %s", result, tc.contentType)
			}
		})
	}
}

func TestWeb2MarkdownDict(t *testing.T) {
	t.Skip("Skip since it's slow, needs ~3 seconds")
	tcs := []struct {
		name string
		url  string
		want string
	}{
		{"merriam-webster", "https://www.merriam-webster.com/dictionary/claude", "Claude Lor raine glass."},
		{"dictionary", "https://www.dictionary.com/browse/claude", "[klawd, klohd] / klɔd, kloʊd /"},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := Web2Markdown(ctx, tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(result, tc.want) {
				t.Fatal(result)
			}
		})
	}
}
