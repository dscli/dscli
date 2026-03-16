package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

// CodeMakeFormat - run code format command provided in the context
// 注意：这里设置ShellStdinKey为os.Stdin是有意的，目的是让mkfmt命令在OSExec中执行，
// 而不是在internal/shell沙箱中执行。因为mkfmt脚本由用户提供，是可信任的，
// 可能包含沙箱不允许的命令，但由于用户指定，是安全的。
func CodeMakeFormat(ctx context.Context) (output string, err error) {
	mkfmt := ContextValue(ctx, MakeFormatKey, "make fmt")
	ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, mkfmt)
	if err != nil {
		err = fmt.Errorf("failed to make code format: %w", err)
	}
	return
}

// CodeMakeFormatWithTimeout - run code format command with timeout
// 提供超时控制，避免格式化命令卡住
func CodeMakeFormatWithTimeout(ctx context.Context, timeout time.Duration) (output string, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return CodeMakeFormat(ctx)
}

// CodeMakeFormatSafe - safe version with default timeout (30 seconds)
// 安全版本，使用默认30秒超时
func CodeMakeFormatSafe(ctx context.Context) (output string, err error) {
	return CodeMakeFormatWithTimeout(ctx, 30*time.Second)
}
