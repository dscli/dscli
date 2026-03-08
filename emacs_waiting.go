package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

// showEmacsAnimation Emacs专用动画
func showEmacsAnimation(ctx context.Context, done chan bool) {
	// Emacs中输出特殊标记，让Emacs可以解析和增强

	// 输出开始标记
	fmt.Println("<!-- DS-CLI-WAITING-START -->")

	// 使用简单的点显示，但输出特殊标记
	ticker := time.NewTicker(500 * time.Millisecond) // Emacs中可以稍快
	defer ticker.Stop()

	dotCount := 0

	for {
		select {
		case <-ctx.Done():
			// 输出结束标记
			fmt.Println("<!-- DS-CLI-WAITING-END -->")
			return
		case <-done:
			// 输出结束标记
			fmt.Println("<!-- DS-CLI-WAITING-END -->")
			return
		case <-ticker.C:
			// 输出一个点和标记
			fmt.Print(".")
			fmt.Printf("<!-- DS-CLI-WAITING-PROGRESS:%d -->", dotCount)
			dotCount++

			// 每10个点换行
			if dotCount >= 10 {
				fmt.Println()
				dotCount = 0
			}
		}
	}
}

// isEmacsEnvironment 检查是否是Emacs环境
func isEmacsEnvironment() bool {
	return os.Getenv("INSIDE_EMACS") != "" || os.Getenv("EMACS") != ""
}
