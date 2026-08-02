#!/usr/bin/env bash
# dscli-flycheck.sh — Run Emacs flycheck on a file via emacsclient
# Usage: dscli-flycheck.sh <file-path> [timeout-seconds]
# Output: raw JSON to stdout (no outer Elisp quoting)
#
# Strategy (same detection as Go's internal/emacsutil):
#   1. emacsclient when an Emacs server is running — reuses the running
#      instance with the user's full configuration.
#   2. Otherwise fall back to a standalone batch Emacs (-q) loading
#      dscli-flycheck.el directly.  Previously the script called
#      emacsclient unconditionally and failed silently (2>/dev/null)
#      when no server existed.
#
# This script lives in the dscli Go project's internal/flycheck/scripts/.
# It finds dscli-flycheck.el via DSCLI_EL_ROOT (set by dscli.el) or
# falls back to walking up from its own location (original dscli.el layout).

set -euo pipefail

FILE="%s"

if [ ! -f "$FILE" ]; then
    echo "{\"error\": \"file not found: $FILE\"}"
    exit 1
fi
ABS_FILE="$(realpath "$FILE")"

# ── 调用 Emacs flycheck ─────────────────────────────────────────────
# 优先 emacsclient：复用运行中的实例（含用户配置）。
# emacsclient --eval 通过 prin1 (Elisp print) 返回结果。
if command -v emacsclient >/dev/null 2>&1 && \
   emacsclient --eval '(server-running-p)' >/dev/null 2>&1; then
    emacsclient --eval "(progn (dscli-flycheck-check-file-json \"$ABS_FILE\"))" 2>/dev/null
    exit $?
fi

# ── 回退：独立 batch emacs（无 server 时）───────────────────────────
# 定位 dscli-flycheck.el。DSCLI_EL_ROOT 由 dscli.el 设置；否则从脚本
# 所在位置向上查找（原始 dscli.el 布局）。注意脚本可能经 mvdan/sh
# 从 stdin 执行，此时 $0 无路径（如 "gosh"），dirname 得 "."——把
# 相对路径解析到 pwd 并防止循环不前进。
EL=""
if [ -n "${DSCLI_EL_ROOT:-}" ] && [ -f "$DSCLI_EL_ROOT/dscli-modules/dscli-flycheck.el" ]; then
    EL="$DSCLI_EL_ROOT/dscli-modules/dscli-flycheck.el"
else
    dir="$(dirname "$0")"
    if [ "$dir" = "." ]; then
        dir="$(pwd)"
    fi
    while [ "$dir" != "/" ]; do
        if [ -f "$dir/dscli-modules/dscli-flycheck.el" ]; then
            EL="$dir/dscli-modules/dscli-flycheck.el"
            break
        fi
        next="$(dirname "$dir")"
        [ "$next" = "$dir" ] && break
        dir="$next"
    done
fi

if command -v emacs >/dev/null 2>&1; then
    if [ -n "$EL" ]; then
        # Note: batch emacs does NOT print --eval results (unlike
        # emacsclient), so wrap with princ to emit the JSON string.
        emacs --batch -q -l "$EL" --eval "(princ (dscli-flycheck-check-file-json \"$ABS_FILE\"))" 2>/dev/null
        exit $?
    fi
    echo "{\"error\": \"no emacs server running and dscli-flycheck.el not found\"}"
    exit 1
fi

echo "{\"error\": \"neither emacsclient nor emacs found in PATH\"}"
exit 1
