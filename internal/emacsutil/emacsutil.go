// Package emacsutil provides detection helpers for Emacs client/server
// availability, shared by wakeup, editor, and flycheck.
//
// The core question every caller asks is the same: "can I reach a
// running Emacs server via emacsclient, or must I start a standalone
// Emacs instance?"  Historically each caller answered it privately;
// this package centralizes the answer so the behavior stays uniform.
package emacsutil

import (
	"context"
	"os/exec"
	"time"
)

// Mode 描述系统上 Emacs 的可用使用方式。
type Mode int

const (
	// ModeNone 表示没有可用的 emacs/emacsclient 二进制。
	ModeNone Mode = iota
	// ModeClientServer 表示 emacsclient 可用且 Emacs server 正在运行。
	// 这是最优方式：复用运行中的实例（含用户配置），开销最小。
	ModeClientServer
	// ModeStandalone 表示 emacs 可用（server 未运行，或没有 emacsclient）。
	// 启动独立实例总是可靠，但会新开一个进程/窗口。
	ModeStandalone
	// ModeClientOnly 表示只有 emacsclient（没有 emacs 二进制）。
	// Last resort：它可能仍能连上其他安装启动的 server。
	ModeClientOnly
)

// Detect 探测系统上 Emacs 的最佳使用方式。
//
// 判定顺序与 wakeup 的历史行为保持一致：
//  1. emacsclient 可用且 server 在跑 → 用 emacsclient（最快、复用配置）
//  2. emacs 可用 → 用独立 emacs（任何情况下都能启动）
//  3. 只有 emacsclient → last resort，仍可能连上 server
//
// 探测本身用 emacsclient（见 ServerRunning）：一次成功的连接同时
// 证明了 emacsclient 在 PATH 且 server 在跑，因此不需要单独的
// executable-find 前置检查——与 dscli-flycheck.sh 的策略完全一致。
func Detect() Mode {
	if ServerRunning() {
		return ModeClientServer
	}
	switch {
	case HasExecutable("emacs"):
		return ModeStandalone
	case HasExecutable("emacsclient"):
		return ModeClientOnly
	}
	return ModeNone
}

// ServerRunning reports whether an Emacs server is accepting client
// connections.  It probes with emacsclient itself: a successful
// connection proves both that emacsclient is on PATH and that a server
// socket exists (the socket lives only while the server runs).
//
// Note: do NOT pass -a/--alternate-editor here.  A non-empty value
// would launch a standalone Emacs when no server exists (side effect),
// and the empty string "" actually makes emacsclient START a daemon and
// wait for it — turning a probe into a state change.  Without -a,
// emacsclient fails fast with exit 1 when the socket is missing (unless
// the user set ALTERNATE_EDITOR, which is rare).  Any failure is
// treated as "not running" so the caller falls back to starting a
// standalone Emacs instance.
func ServerRunning() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "emacsclient", "--eval", "(server-running-p)")
	return cmd.Run() == nil
}

// HasExecutable checks if a command is available in PATH.
func HasExecutable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
