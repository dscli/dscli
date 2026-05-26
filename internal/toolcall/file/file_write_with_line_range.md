# write_file_with_line_range

Write file content with line range.

Write content to a specific line range in a file. Supports:

1. Replace: overwrite the specified line range with new content

2. Delete: set content to empty string to remove lines

3. Insert: when start_line exceeds file length, append at end

4. Create: create a new file if it doesn't exist

**CAS tag verification** (optional): pass line_tag (single)
or line_tags (multi,

-separated) from read_file output
to prevent writing when file content has changed. Tags are
verified against current file content before applying changes.
If any tag mismatches, the write is rejected with actual content.

Best for non-code files (configs, docs) needing precise line
control.

context (default true): after editing, returns a context
window showing the file state around the edit. Set false
to suppress and save output tokens.

Examples:
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="new")
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="")
  write_file_with_line_range(path="file.txt", start_line=5, content="new")
  write_file_with_line_range(path="file.txt", start_line=5, content="new")
  write_file_with_line_range(path="file.txt", start_line=10, line_tag="Q8fA", content="int count = 11;")
  write_file_with_line_range(path="file.txt", start_line=11, line_tags="rA3_
Kq9z
PX0b", content="if (count > limit)
    return limit;")
