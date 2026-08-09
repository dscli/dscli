package toolcall

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
)

// toolProgress 记录 HandleToolCalls 中已完成的工具调用数（结果已落库）。
// 中断信号处理器据此判断已完成/未完成的边界。
var toolProgress atomic.Int64

// InstallInterruptHandler 在工具执行期间注册中断信号处理。
//
// 用户主动停止（Ctrl+C = SIGINT、kill = SIGTERM）时进程默认直接终止，
// 数据库最后一条仍是带 tool_calls 的 assistant 消息，下次启动会错误重放
// 已取消的工具调用。本处理器在信号到达时先修正数据库状态
// （prompt.MarkInterruptedToolCalls）再退出，使重启后不再重放。
//
// SIGHUP（终端关闭）未注册：syscall.SIGHUP 在 Windows 上不可用；
// 该场景与 SIGKILL 一样退回"重启重放 + 提示"兜底（见 ChatRunE）。
//
// 返回的 stop 函数注销信号监听，应在工具执行结束后调用。
func InstallInterruptHandler(ctx context.Context) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	var closeOnce sync.Once
	go func() {
		select {
		case <-ch:
			handleInterrupt(ctx, os.Exit)
		case <-done:
		}
	}()
	return func() {
		signal.Stop(ch)
		closeOnce.Do(func() { close(done) })
	}
}

// handleInterrupt 标记未完成的工具调用并以 130（128+SIGINT 惯例）退出。
// exit 参数可注入以便测试。
func handleInterrupt(ctx context.Context, exit func(int)) {
	completed := int(toolProgress.Load())
	n, err := prompt.MarkInterruptedToolCalls(ctx, completed)
	if err != nil {
		outfmt.Error("failed to mark interrupted tool calls: %v", err)
	} else if n > 0 {
		outfmt.Warn("⚠️ 已中断 %d 个未完成的工具调用，下次启动不会重放", n)
	}
	exit(130)
}
