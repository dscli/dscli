# write_file_with_line_range

Write content to a line range in a file; empty content deletes lines.
Modes:
1. Replace: overwrite [start_line, end_line]
2. Delete: content="" removes the range
3. Insert: insert_before_line=N puts content BEFORE line N (N=total+1 appends at end); exclusive with start_line/end_line/line_tags; pair with line_tag for CAS check
4. Create: missing file is created (must start at line 1)

**Safety**: omit end_line → edit start_line only; end_line=-1 replaces to EOF.

**CAS tags**: pass line_tag (single) or line_tags (newline-separated) from read_file output; any mismatch rejects the write.

**Length-mismatch guard** (no tags): content much larger than the replaced region (≥3× and ≥10 extra lines) returns a warning — typical sign of misaligned line numbers when insert was intended. Write still proceeds; tags silence it.

Examples:
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="new")   — replace lines 5-10
  write_file_with_line_range(path="file.txt", start_line=5, content="new")                 — replace line 5 only
  write_file_with_line_range(path="file.txt", start_line=5, end_line=-1, content="new")    — replace line 5 to end
  write_file_with_line_range(path="file.txt", start_line=10, line_tag="Q8fA", content="int count = 11;")
  write_file_with_line_range(path="file.txt", start_line=11, line_tags="rA3_
Kq9z
PX0b", content="if (count > limit)
    return limit;")
  write_file_with_line_range(path="file.txt", insert_before_line=3, content="new line before 3")   — insert before line 3
  write_file_with_line_range(path="file.txt", insert_before_line=4, line_tag="Q8fA", content="new")  — insert with CAS check of line 4
