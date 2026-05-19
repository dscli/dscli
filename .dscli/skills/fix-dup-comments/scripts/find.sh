#!/usr/bin/env bash
# find.sh — find duplicate consecutive comment lines in Go source files.
# Detects:
#   1. Single-line: same comment repeated on next line (within a block)
#   2. Multi-line block: entire adjacent comment blocks repeated
# Usage: bash find.sh [dir]
#   dir — directory to scan (default: .)
set -euo pipefail

DIR="${1:-.}"

find "$DIR" -name '*.go' -not -path '*/.git/*' \
  -exec awk '
    function flush_block(reset_prev) {
      if (block_len == 0) return
      if (block_len == prev_len && prev_len > 0) {
        same = 1
        for (i = 1; i <= block_len; i++) {
          if (block[i] != prev[i]) { same = 0; break }
        }
        if (same) {
          for (i = 1; i <= block_len; i++)
            print fn ":" block_ln[i] ":" block[i]
        }
      }
      for (i = 1; i <= block_len; i++) { prev[i] = block[i]; prev_ln[i] = block_ln[i] }
      prev_len = block_len
      block_len = 0
      if (reset_prev) prev_len = 0
    }

    FILENAME != fn {
      flush_block(1)
      fn = FILENAME
    }

    # Comment line
    /^[[:space:]]*\/\// {
      # Single-line duplicate: same as previous line
      if ($0 == last_line)
        print fn ":" FNR ":" $0
      last_line = $0
      block_len++
      block[block_len] = $0
      block_ln[block_len] = FNR
      next
    }

    # Blank line: flush block, keep prev for multi-block compare
    /^[[:space:]]*$/ { last_line = $0; flush_block(0); next }

    # Code line: flush and reset
    { last_line = $0; flush_block(1) }
  ' {} +
