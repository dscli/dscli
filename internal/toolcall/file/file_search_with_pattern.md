Search file by pattern with context lines

Search for lines matching a pattern in a file, showing
surrounding context.

Best for non-code files: logs, configs, etc. Supports
case-sensitive, max matches.

Examples:
  search_file_with_pattern(path="app.log", pattern="error")
  search_file_with_pattern(path="main.go", pattern="TODO", context_lines="3")
