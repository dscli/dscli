package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientWithRetry(t *testing.T) {
	// 测试创建带重试的客户端
	client := NewClientWithRetry("test-key", "https://api.example.com", 3, 60*time.Second)

	// 验证客户端类型
	if _, ok := client.(*Deepseek); !ok {
		t.Errorf("Expected *Deepseek, got %T", client)
	}

	// 验证最大重试次数
	dsClient := client.(*Deepseek)
	if dsClient.maxRetries != 3 {
		t.Errorf("Expected maxRetries=3, got %d", dsClient.maxRetries)
	}

	if dsClient.retryDelay != 60*time.Second {
		t.Errorf("Expected retryDelay=60s, got %v", dsClient.retryDelay)
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

	// 创建带重试的客户端
	client := NewClientWithRetry("test-key", server.URL, 3, 100*time.Millisecond).(*Deepseek)

	// 发送请求
	var result map[string]string
	err := client.doRequestWithRetry("GET", "/test", nil, &result)
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

	// 创建带重试的客户端（最大重试2次）
	client := NewClientWithRetry("test-key", server.URL, 2, 100*time.Millisecond).(*Deepseek)

	// 发送请求
	var result map[string]string
	err := client.doRequestWithRetry("GET", "/test", nil, &result)

	if err == nil {
		t.Error("Expected error after max retries, got nil")
	}

	// 应该尝试了3次（初始请求 + 2次重试）
	if attempts != 3 {
		t.Errorf("Expected 3 attempts (initial + 2 retries), got %d", attempts)
	}

	// 检查错误消息
	expectedErr := "经过2次重试后仍然失败"
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

	// 创建带重试的客户端
	client := NewClientWithRetry("test-key", server.URL, 3, 100*time.Millisecond).(*Deepseek)

	// 发送请求
	var result map[string]string
	err := client.doRequestWithRetry("GET", "/test", nil, &result)

	if err == nil {
		t.Error("Expected error for bad request, got nil")
	}

	// 400错误不可重试，应该只尝试1次
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retry for 400), got %d", attempts)
	}
}
