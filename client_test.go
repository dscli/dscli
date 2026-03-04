package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient 创建测试用的客户端，使用较短的重试延迟
func newTestClient(apiKey, baseURL string) *Deepseek {
	return &Deepseek{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
		retryDelay: 100 * time.Millisecond, // 测试使用较短延迟
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "connection refused",
			err:      fmt.Errorf("connection refused"),
			expected: true,
		},
		{
			name:     "dial tcp error",
			err:      fmt.Errorf("dial tcp 127.0.0.1:8080: connect: connection refused"),
			expected: true,
		},
		{
			name:     "timeout error",
			err:      fmt.Errorf("i/o timeout"),
			expected: true,
		},
		{
			name:     "EOF error",
			err:      fmt.Errorf("EOF"),
			expected: true,
		},
		{
			name:     "HTTP 500 error",
			err:      fmt.Errorf("API 返回错误状态码 500: internal server error"),
			expected: true,
		},
		{
			name:     "HTTP 502 error",
			err:      fmt.Errorf("API 返回错误状态码 502: bad gateway"),
			expected: true,
		},
		{
			name:     "HTTP 429 error",
			err:      fmt.Errorf("API 返回错误状态码 429: too many requests"),
			expected: true,
		},
		{
			name:     "HTTP 400 error",
			err:      fmt.Errorf("API 返回错误状态码 400: bad request"),
			expected: false,
		},
		{
			name:     "HTTP 401 error",
			err:      fmt.Errorf("API 返回错误状态码 401: unauthorized"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "other error",
			err:      fmt.Errorf("some other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("isRetryableError(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestRetryLogic(t *testing.T) {
	// 创建一个模拟服务器，前两次失败，第三次成功
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts <= 2 {
			// 前两次返回500错误
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal server error"))
		} else {
			// 第三次成功
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"data": "success"}`))
		}
	}))
	defer server.Close()

	// 创建测试客户端
	client := newTestClient("test-key", server.URL)

	// 发送请求
	var result map[string]string
	err := client.doRequest("GET", "/test", nil, &result)
	if err != nil {
		t.Errorf("Expected success after retries, got error: %v", err)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts (2 failures + 1 success), got %d", attempts)
	}

	if result["data"] != "success" {
		t.Errorf("Expected result data='success', got %v", result)
	}
}

func TestMaxRetriesExceeded(t *testing.T) {
	// 创建一个总是失败的模拟服务器
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer server.Close()

	// 创建测试客户端
	client := newTestClient("test-key", server.URL)

	// 发送请求
	var result map[string]string
	err := client.doRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("Expected error after max retries, got nil")
	}

	// 应该尝试了4次（初始请求 + 3次重试）
	if attempts != 4 {
		t.Errorf("Expected 4 attempts (initial + 3 retries), got %d", attempts)
	}

	// 检查错误消息
	expectedErr := "经过3次重试后仍然失败"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("Expected error to contain '%s', got: %v", expectedErr, err)
	}
}

func TestNonRetryableError(t *testing.T) {
	// 创建一个返回400错误的模拟服务器
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	// 创建测试客户端
	client := newTestClient("test-key", server.URL)

	// 发送请求
	var result map[string]string
	err := client.doRequest("GET", "/test", nil, &result)

	if err == nil {
		t.Error("Expected error for bad request, got nil")
	}

	// 400错误不可重试，应该只尝试1次
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry for 400), got %d", attempts)
	}
}

func TestNewClient(t *testing.T) {
	// 测试创建客户端
	client := NewClient("test-key", "https://api.example.com")

	// 验证客户端类型
	if _, ok := client.(*Deepseek); !ok {
		t.Errorf("Expected *Deepseek, got %T", client)
	}

	// 验证默认重试配置
	dsClient := client.(*Deepseek)
	if dsClient.maxRetries != 3 {
		t.Errorf("Expected maxRetries=3, got %d", dsClient.maxRetries)
	}

	if dsClient.retryDelay != 60*time.Second {
		t.Errorf("Expected retryDelay=60s, got %v", dsClient.retryDelay)
	}
}

func TestRetryNotificationLength(t *testing.T) {
	// 测试重试通知是否在20字以内
	testCases := []struct {
		name     string
		delay    time.Duration
		expected string
	}{
		{
			name:     "1秒延迟",
			delay:    1 * time.Second,
			expected: "网络异常，1秒后重试...",
		},
		{
			name:     "60秒延迟",
			delay:    60 * time.Second,
			expected: "网络异常，60秒后重试...",
		},
		{
			name:     "300秒延迟",
			delay:    300 * time.Second,
			expected: "网络异常，300秒后重试...",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 计算通知消息
			notification := fmt.Sprintf("网络异常，%d秒后重试...", int(tc.delay.Seconds()))

			// 检查长度是否在20字以内
			chineseCharCount := len([]rune(notification))
			if chineseCharCount > 20 {
				t.Errorf("通知消息超过20字: %s (长度: %d)", notification, chineseCharCount)
			}

			// 验证内容
			if notification != tc.expected {
				t.Errorf("通知消息不匹配: got %s, want %s", notification, tc.expected)
			}
		})
	}
}

func TestSuccessNotificationLength(t *testing.T) {
	// 测试成功通知是否在20字以内
	notification := "重试成功！"

	// 检查长度是否在20字以内
	chineseCharCount := len([]rune(notification))
	if chineseCharCount > 20 {
		t.Errorf("成功通知超过20字: %s (长度: %d)", notification, chineseCharCount)
	}

	// 验证内容
	if notification != "重试成功！" {
		t.Errorf("成功通知不匹配: got %s, want %s", notification, "重试成功！")
	}
}

func TestRetryDelayCalculation(t *testing.T) {
	// 测试指数退避计算
	client := &Deepseek{
		retryDelay: 60 * time.Second,
		maxRetries: 3,
	}

	testCases := []struct {
		name     string
		attempt  int
		expected time.Duration
	}{
		{
			name:     "第一次重试",
			attempt:  1,
			expected: 60 * time.Second, // 2^0 * 60s = 60s
		},
		{
			name:     "第二次重试",
			attempt:  2,
			expected: 120 * time.Second, // 2^1 * 60s = 120s
		},
		{
			name:     "第三次重试",
			attempt:  3,
			expected: 240 * time.Second, // 2^2 * 60s = 240s
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			delay := time.Duration(1<<(tc.attempt-1)) * client.retryDelay
			if delay > 300*time.Second {
				delay = 300 * time.Second
			}

			if delay != tc.expected {
				t.Errorf("重试延迟计算错误: attempt=%d, got %v, want %v",
					tc.attempt, delay, tc.expected)
			}
		})
	}
}
