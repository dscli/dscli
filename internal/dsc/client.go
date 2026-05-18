package dsc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

// httpClient 单例HTTP客户端，避免创建多个连接池
// 超时设置为10分钟，适应DeepSeek API长处理时间的特性
var httpClient = &http.Client{
	Timeout: 600 * time.Second,
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// 上下文超时（绝对可重试）—— 覆盖 io.ReadAll 读响应体超时、
	// httpClient.Do 超时等场景。errors.Is 沿 %w 链自动解包，
	// 即使错误被多层 fmt.Errorf 包装也能精准命中。
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	errStr := err.Error()

	// 网络连接错误（可重试）
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "TLS handshake timeout") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection timed out") {
		return true
	}

	// 5xx 服务端错误（可重试）—— 服务端临时故障，重试是唯一选择。
	// 不再要求错误消息必须包含 "API 返回错误状态码" 前缀：
	// 去掉外层守卫，避免重试逻辑绑定到具体措辞。
	// "500"/"502"/"503"/"504" 这些数字在非 HTTP 上下文中
	// 极少出现，且即使误判，重试天花板（maxRetries=600）
	// 也提供了安全网。
	if strings.Contains(errStr, "500") ||
		strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") ||
		strings.Contains(errStr, "504") ||
		strings.Contains(errStr, "429") { // 429 Too Many Requests 也可重试
		return true
	}

	return false
}

// doRequestSingle 单次请求（不带重试）
func (c *Deepseek) doRequestSingle(method, path string, body, result any) (err error) {
	url := c.baseURL + path

	var reqBody io.Reader
	var data []byte
	if body != nil {
		data, err = outfmt.JSONMarshal(body)
		if err != nil {
			err = fmt.Errorf("序列化请求失败: %w", err)
			return err
		}

		defer outfmt.DebugBytes("json", data)

		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		err = fmt.Errorf("创建请求失败: %w", err)
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		// 检查是否是网络错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			err = fmt.Errorf("网络请求超时: %w", err)
		} else {
			err = fmt.Errorf("网络请求失败: %w", err)
		}
		return err
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("读取响应失败: %w", err)
		return err
	}

	defer outfmt.DebugBytes("", respBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
		return err
	}

	if result != nil {
		if err = json.Unmarshal(respBody, result); err != nil {
			err = fmt.Errorf("解析响应失败: %w", err)
			return err
		}
	}
	return err
}

// doRequest 请求方法（自动重试）
func (c *Deepseek) doRequest(method, path string, body, result any) (err error) {
	defer outfmt.StartWaiting(time.Second * 3)()
	var lastErr error
	attempt := 0
	for attempt = 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 计算重试延迟（指数退避）
			delay := min(time.Duration(1<<(attempt-1))*c.retryDelay,
				300*time.Second)
			// 简洁通知用户（不超过20字）
			if delay.Seconds() < 1 {
				outfmt.Notice("网络异常，立即重试...")
			} else {
				outfmt.Notice("网络异常，%d秒后重试...", int(delay.Seconds()))
			}
			time.Sleep(delay)
		}

		err = c.doRequestSingle(method, path, body, result)
		lastErr = err

		if lastErr == nil {
			// 成功，返回
			if attempt > 0 {
				outfmt.Notice("重试成功")
			}
			return nil
		}

		// 检查错误是否可重试
		if !isRetryableError(lastErr) {
			// 不可重试的错误，直接返回
			return lastErr
		}
	}

	// 超过最大重试次数
	return fmt.Errorf("经过%d次重试后仍然失败: %w", c.maxRetries, lastErr)
}
