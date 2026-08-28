# write_file

Write content to a file. Two modes, selected by parameters:

**Mode 1 — full file (no line parameters)**: overwrite the whole file
(or create it, creating parent directories). POSIX trailing newline ensured.

  write_file(path="main.go", content="package main\n...")

**Mode 2 — line-range edit (any line parameter)**: replace/delete/insert lines;
content="" deletes the range. Create: missing file is created (must start line 1).

**Safety**: omit end_line → edit start_line only; end_line=-1 replaces to EOF.

**CAS tags**: pass line_tag (single) or line_tags (newline-separated) from
read_file output; any mismatch rejects the write before any change.

**Length-mismatch guard** (no tags): content ≥3× and ≥10 lines larger than the
replaced region warns — typical sign of misaligned line numbers when insert was
intended. Write still proceeds; tags silence it.

Content max 524288 chars; split into multiple calls for larger content
(continue with insert_before_line=total+1, or read_file to find the total).

context (default true): after writing, returns a context window around the
edit. Set false to suppress and save output tokens.

Examples:
  write_file(path="file.txt", content="line 1\nline 2")                    — overwrite whole file
  write_file(path="file.txt", start_line=5, end_line=10, content="new")   — replace lines 5-10
  write_file(path="file.txt", start_line=5, content="new")                 — replace line 5 only
  write_file(path="file.txt", start_line=5, end_line=-1, content="new")    — replace line 5 to end
  write_file(path="file.txt", start_line=10, line_tag="Q8fA", content="int count = 11;")
  write_file(path="file.txt", insert_before_line=3, content="new line before 3")  — insert before line 3
