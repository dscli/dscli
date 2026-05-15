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

ABS_FILE="$(realpath "$FILE")"

# ── 调用 Emacs flycheck ─────────────────────────────────────────────
# emacsclient --eval 通过 prin1 (Elisp print) 返回结果，
# 外层包含 Elisp 转义的字符串引号。内层 python3 剥离外层后输出裸 JSON。
emacsclient --eval "(progn (dscli-flycheck-check-file-json \"$ABS_FILE\" $TIMEOUT))" 2>/dev/null
