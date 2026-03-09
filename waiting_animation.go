package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// WaitingManager 等待动画管理器
type WaitingManager struct {
	mu            sync.RWMutex
	animationCtx  context.Context
	cancelFunc    context.CancelFunc
	done          chan bool
	active        bool
	lastOutput    time.Time
	startTime     time.Time
	outputWriter  io.Writer
	animationType string // "emacs", "terminal", "plain"
}

var (
	waitingManager *WaitingManager
	managerOnce    sync.Once
)

// GetWaitingManager 获取等待动画管理器单例
func GetWaitingManager() *WaitingManager {
	managerOnce.Do(func() {
		waitingManager = &WaitingManager{
			done:         make(chan bool),
			outputWriter: os.Stdout,
		}
	})
	return waitingManager
}

// StartWaiting 开始等待监控
// 如果 delay 秒内没有输出，则启动等待动画
func (w *WaitingManager) StartWaiting(delay time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.active {
		return
	}

	w.active = true
	w.lastOutput = time.Now()
	w.startTime = time.Now()

	// 确定动画类型
	w.animationType = w.detectAnimationType()

	// 创建动画上下文
	ctx := context.Background()
	w.animationCtx, w.cancelFunc = context.WithCancel(ctx)

	// 启动监控goroutine
	go w.monitorAndStartAnimation(delay)
}

// StopWaiting 停止等待动画
func (w *WaitingManager) StopWaiting() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.active {
		return
	}

	w.active = false
	if w.cancelFunc != nil {
		w.cancelFunc()
	}
	select {
	case w.done <- true:
	default:
	}
}

// RecordOutput 记录输出事件，重置等待计时器
func (w *WaitingManager) RecordOutput() {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if w.active {
		w.lastOutput = time.Now()
	}
}

// IsActive 检查等待动画是否活跃
func (w *WaitingManager) IsActive() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.active
}

// isTerminal 简单判断是否是终端环境
func isTerminal() bool {
	// 超简单判断：只有在非常明确是交互式终端时才使用动画

	// 1. 检查标准输出是否是终端设备
	if fileInfo, err := os.Stdout.Stat(); err != nil || (fileInfo.Mode()&os.ModeCharDevice) == 0 {
		return false // 不是终端设备
	}

	// 2. 检查是否是哑终端
	if term := os.Getenv("TERM"); term == "dumb" {
		return false
	}

	// 3. 排除Emacs环境（最可能出问题的环境）
	if os.Getenv("INSIDE_EMACS") != "" || os.Getenv("EMACS") != "" {
		return false
	}

	// 其他情况认为是终端
	return true
}

// detectAnimationType 检测动画类型
func (w *WaitingManager) detectAnimationType() string {
	// 1. 检查是否是Emacs环境
	if isEmacsEnvironment() {
		return "emacs"
	}

	// 2. 检查是否是终端环境
	if isTerminal() {
		return "terminal"
	}

	// 3. 默认使用简单动画
	return "plain"
}

// monitorAndStartAnimation 监控并启动动画
func (w *WaitingManager) monitorAndStartAnimation(delay time.Duration) {
	// 等待指定的延迟时间
	select {
	case <-time.After(delay):
		// 延迟时间到，检查是否需要启动动画
		w.mu.RLock()
		needsAnimation := time.Since(w.lastOutput) >= delay
		w.mu.RUnlock()

		if needsAnimation {
			w.startAnimation()
		} else {
			// 在延迟期间有输出，不需要动画
			w.mu.Lock()
			w.active = false
			w.mu.Unlock()
		}
	case <-w.animationCtx.Done():
		// 在延迟期间被取消
		w.mu.Lock()
		w.active = false
		w.mu.Unlock()
		return
	}
}

// showTerminalAnimation 在终端中显示动画（使用回显）
func showTerminalAnimation(ctx context.Context, done chan bool) {
	// 旋转动画字符
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	idx := 0

	ticker := time.NewTicker(1000 * time.Millisecond)
	defer ticker.Stop()

	// 显示初始动画
	fmt.Print(spinner[idx])

	for {
		select {
		case <-ctx.Done():
			// 清除动画
			fmt.Print("\r")
			return
		case <-done:
			// 清除动画
			fmt.Print("\r")
			return
		case <-ticker.C:
			// 清除上一帧
			fmt.Print("\r")

			// 显示下一帧
			idx = (idx + 1) % len(spinner)
			fmt.Print(spinner[idx])
		}
	}
}

// showPlainAnimation 在非终端环境中显示简单点
func showPlainAnimation(ctx context.Context, done chan bool) {
	// 简单的等待提示：每3秒打印一个点
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	dotCount := 0

	// 先输出一个换行，确保点从新行开始
	fmt.Println()

	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			// 打印一个点
			fmt.Print(".")
			dotCount++

			// 每10个点换行，避免一行太长
			if dotCount >= 10 {
				fmt.Println()
				dotCount = 0
			}
		}
	}
}

// startAnimation 启动动画
func (w *WaitingManager) startAnimation() {
	switch w.animationType {
	case "emacs":
		go showEmacsAnimation(w.animationCtx, w.done)
	case "terminal":
		go showTerminalAnimation(w.animationCtx, w.done)
	case "plain":
		go showPlainAnimation(w.animationCtx, w.done)
	}
}
