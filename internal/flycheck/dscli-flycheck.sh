#!/usr/bin/env bash
# dscli-flycheck.sh — Run Emacs flycheck on a file via emacsclient
# Usage: dscli-flycheck.sh <file-path> [timeout-seconds]
# Output: raw JSON to stdout (no outer Elisp quoting)
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
# emacsclient --eval 通过 prin1 (Elisp print) 返回结果。
emacsclient --eval "(progn (dscli-flycheck-check-file-json \"$ABS_FILE\"))" 2>/dev/null
