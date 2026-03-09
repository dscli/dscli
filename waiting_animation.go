package main

import (
	"context"
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

// WithWaiting 包装一个函数，使其支持等待动画
func WithWaiting(delay time.Duration, fn func() error) error {
	manager := GetWaitingManager()
	manager.StartWaiting(delay)
	defer manager.StopWaiting()

	return fn()
}

// WaitingPrintln 支持等待动画的 Println
func WaitingPrintln(a ...any) (n int, err error) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	return Println(a...)
}

// WaitingPrintf 支持等待动画的 Printf
func WaitingPrintf(format string, a ...any) (n int, err error) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	return Printf(format, a...)
}

// WaitingInfo 支持等待动画的 Info
func WaitingInfo(format string, a ...any) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	Info(format, a...)
}

// WaitingDebug 支持等待动画的 Debug
func WaitingDebug(format string, a ...any) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	Debug(format, a...)
}

// WaitingWarn 支持等待动画的 Warn
func WaitingWarn(format string, a ...any) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	Warn(format, a...)
}

// WaitingError 支持等待动画的 Error
func WaitingError(format string, a ...any) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	Error(format, a...)
}

// WaitingSuccess 支持等待动画的 Success
func WaitingSuccess(format string, a ...any) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	Success(format, a...)
}

// WaitingNotice 支持等待动画的 Notice
func WaitingNotice(format string, a ...any) {
	manager := GetWaitingManager()
	manager.RecordOutput()
	Notice(format, a...)
}
