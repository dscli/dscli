Search code text with structure-aware context

Search for patterns in code files, showing matches with
function/class context info.

Supports: single file, wildcards (*.go), multiple files,
current dir (.), recursive (**/*.go).

Examples:
  search_code_semantic(file_pattern="*.go", search_pattern="error")
  search_code_semantic(file_pattern="main.go root.go", search_pattern="TODO", context_lines="3")
