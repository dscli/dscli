Run bash script.

Executes a shell script via mvdan/sh. Returns three signals:
- result — stdout (green, script succeeded)
- warning — stderr (yellow, succeeded with diagnostics)
- error — failure (red, script failed)

A "result with warning" (yellow) means the script succeeded
but produced stderr output (warnings, progress, etc.).

Output format:
- Success: formatted text with execution result and statistics
- Failure: formatted text with error info, output content, and
  execution statistics

Examples:
  1. Bash: echo "Hello"
  2. Shell: ls -la
  3. Files: cat file.txt
  4. Git: git status

Caution: Avoid destructive operations.
