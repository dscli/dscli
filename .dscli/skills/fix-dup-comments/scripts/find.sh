#!/usr/bin/env bash
# find.sh — find duplicate consecutive comment lines in Go source files.
# Usage: bash find.sh [dir]
#   dir — directory to scan (default: .)
set -euo pipefail

DIR="${1:-.}"

find "$DIR" -name '*.go' -not -path '*/.git/*' \
  -exec awk 'FNR==1{prev="";fn=FILENAME}
    /^[[:space:]]*\/\// && $0==prev {print fn":"FNR":"$0}
    {prev=$0}' {} +
