Write file content with line range.

Write content to a specific line range in a file. Supports:
1. Replace: overwrite the specified line range with new content
2. Delete: set content to empty string to remove lines
3. Insert: when start_line exceeds file length, append at end
4. Create: create a new file if it doesn't exist

Best for non-code files (configs, docs) needing precise line
control.

Examples:
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="new")
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="")
  write_file_with_line_range(path="file.txt", start_line=5, content="new")
