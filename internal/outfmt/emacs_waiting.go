package outfmt

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"
)

// showEmacsAnimation Emacs专用动画
func showEmacsAnimation(ctx context.Context, done chan bool) {
	// Emacs中输出特殊标记，让Emacs可以解析和增强

	// 从环境变量读取动画频率（秒）
	animationInterval := getEmacsAnimationInterval()

	// 输出开始标记
	fmt.Println("<!-- DS-CLI-WAITING-START -->")

	// 创建带超时的上下文，避免动画永远运行
	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute) // 5分钟超时
	defer cancel()

	// 使用配置的频率
	ticker := time.NewTicker(animationInterval)
	defer ticker.Stop()

	progressCount := 0

	for {
		select {
		case <-timeoutCtx.Done():
			// 超时或取消：输出结束标记
			fmt.Println("<!-- DS-CLI-WAITING-END -->")
			fmt.Println("<!-- DS-CLI-WAITING-TIMEOUT -->")
			return
		case <-ctx.Done():
			// 上下文取消：输出结束标记
			fmt.Println("<!-- DS-CLI-WAITING-END -->")
			fmt.Println("<!-- DS-CLI-WAITING-CANCELLED -->")
			return
		case <-done:
			// 正常完成：输出结束标记
			fmt.Println("<!-- DS-CLI-WAITING-END -->")
			fmt.Println("<!-- DS-CLI-WAITING-COMPLETED -->")
			return
		case <-ticker.C:
			// 只输出标记，不输出点字符
			fmt.Printf("<!-- DS-CLI-WAITING-PROGRESS:%d -->\n", progressCount)
			progressCount++

			// 每10次进度输出一个状态标记
			if progressCount%10 == 0 {
				fmt.Printf("<!-- DS-CLI-WAITING-STATUS:progress=%d -->\n", progressCount)
			}
		}
	}
}

// getEmacsAnimationInterval 从环境变量获取Emacs动画间隔
func getEmacsAnimationInterval() time.Duration {
	// 默认间隔：1秒
	defaultInterval := time.Second

	// 尝试从环境变量读取
	envVars := []string{"EMACS", "INSIDE_EMACS"}
	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			// 尝试解析为整数（秒）
			if seconds, err := strconv.Atoi(value); err == nil && seconds > 0 {
				return time.Duration(seconds) * time.Second
			}
		}
	}

	return defaultInterval
}

// isEmacsEnvironment 检查是否是Emacs环境
func isEmacsEnvironment() bool {
	return os.Getenv("INSIDE_EMACS") != "" || os.Getenv("EMACS") != ""
}

// isEmacsEnvironmentWithConfig 检查是否是Emacs环境并返回配置信息
func isEmacsEnvironmentWithConfig() (bool, time.Duration) {
	isEmacs := isEmacsEnvironment()
	interval := getEmacsAnimationInterval()
	return isEmacs, interval
}
