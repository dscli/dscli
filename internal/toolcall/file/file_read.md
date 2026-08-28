# read_file

Read file content, optionally a line range. awk-compatible output.

Read the whole file (default), or a 1-based inclusive line range:
  read_file(path="main.go")
  read_file(path="main.go", start_line=10, end_line=20)

Output format matches:
  awk 'NR>=start && NR<=end {print NR": "$0}'

Each line includes a 4-character checksum tag for CAS
(check-and-set) safety:
  10:[Q8fA] int count = 10;

These tags can be passed to write_file as line_tag or line_tags
to prevent editing stale content.

Best for non-code files (configs, docs) needing precise line
control.

Examples:
  read_file(path="file.txt")
  read_file(path="file.txt", start_line=3, end_line=3)
  read_file(path="file.txt", start_line=10, end_line=20)
  read_file(path="file.txt", start_line=50)
