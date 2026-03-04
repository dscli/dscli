package main

import (
	"os"
	"strconv"
	"time"
)

// initRetryConfig 初始化重试配置
func initRetryConfig() (maxRetries int, retryDelay time.Duration) {
	// 默认配置
	maxRetries = 3                // 默认重试3次
	retryDelay = 60 * time.Second // 默认重试延迟60秒

	// 从环境变量读取配置
	if retryStr := os.Getenv("DEEPSEEK_MAX_RETRIES"); retryStr != "" {
		if n, err := strconv.Atoi(retryStr); err == nil && n >= 0 {
			maxRetries = n
		}
	}

	if delayStr := os.Getenv("DEEPSEEK_RETRY_DELAY"); delayStr != "" {
		if n, err := strconv.Atoi(delayStr); err == nil && n > 0 {
			retryDelay = time.Duration(n) * time.Second
		}
	}

	return maxRetries, retryDelay
}

// createClientWithRetry 创建带重试的客户端
func createClientWithRetry(apiKey, baseURL string) Client {
	maxRetries, retryDelay := initRetryConfig()
	return NewClientWithRetry(apiKey, baseURL, maxRetries, retryDelay)
}
