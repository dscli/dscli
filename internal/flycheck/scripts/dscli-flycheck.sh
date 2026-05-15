#!/usr/bin/env bash
# dscli-flycheck.sh — Run Emacs flycheck on a file via emacsclient
# Usage: dscli-flycheck.sh <file-path> [timeout-seconds]
# Output: raw JSON to stdout (no outer Elisp quoting)
#
# This script lives in the dscli Go project's internal/flycheck/scripts/.
# It finds dscli-flycheck.el via DSCLI_EL_ROOT (set by dscli.el) or
# falls back to walking up from its own location (original dscli.el layout).

set -euo pipefail

FILE="$1"
TIMEOUT="${2:-30}"

if [ ! -f "$FILE" ]; then
    echo "{\"error\": \"file not found: $FILE\"}"
    exit 1
fi

# ── 查找 dscli-flycheck.el ──────────────────────────────────────────
# 优先使用 DSCLI_EL_ROOT（dscli.el 启动进程时设置）
# 回退：从脚本所在位置向上查找（兼容 dscli.el 项目内部调用）
MODULE_PATH=""
if [ -n "${DSCLI_EL_ROOT:-}" ]; then
    MODULE_PATH="$DSCLI_EL_ROOT/dscli-modules/dscli-flycheck.el"
elif [ -n "${BASH_SOURCE:-}" ]; then
    # BASH_SOURCE is more reliable than $0 when piped via stdin
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    PROJECT_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || \
        dirname "$(dirname "$(dirname "$(dirname "$SCRIPT_DIR")")")")"
    MODULE_PATH="$PROJECT_ROOT/dscli-modules/dscli-flycheck.el"
fi

if [ ! -f "$MODULE_PATH" ]; then
    echo "{\"error\": \"dscli-flycheck.el not found at: $MODULE_PATH. Set DSCLI_EL_ROOT to the dscli.el project root.\"}"
    exit 1
fi

ABS_FILE="$(realpath "$FILE")"

# ── 调用 Emacs flycheck ─────────────────────────────────────────────
# emacsclient --eval 通过 prin1 (Elisp print) 返回结果，
# 外层包含 Elisp 转义的字符串引号。内层 python3 剥离外层后输出裸 JSON。
emacsclient --eval "(progn (load-file \"$MODULE_PATH\") (dscli-flycheck-check-file-json \"$ABS_FILE\" $TIMEOUT))" 2>/dev/null \
    | python3 -c "
import sys, json
raw = sys.stdin.read().strip()
if not raw:
    sys.exit(0)
inner = json.loads(raw)
print(inner, end='')
"
