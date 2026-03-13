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

func Test_isRetryableError(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		err  error
		want bool
	}{
		{
			"400",
			fmt.Errorf(`API 返回错误状态码 400: {"error":{"message":"An assistant message with 'tool_calls' must be followed by tool messages responding to each 'tool_call_id'. (insufficient tool messages following tool_calls message)","type":"invalid_request_error","param":null,"code":"invalid_request_error"}
`),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableError(tt.err)
			if got != tt.want {
				t.Errorf("isRetryableError() = %v, want %v", got, tt.want)
			}
		})
	}
}
