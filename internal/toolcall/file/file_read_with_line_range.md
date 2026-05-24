# read_file_with_line_range

Read file line range, awk-compatible output.

Read specific lines from a file. Output format matches:
  awk 'NR>=start && NR<=end {print NR": "$0}'

Each line includes a 4-character checksum tag for CAS
(check-and-set) safety:
  10:Q8fA int count = 10;

These tags can be passed to write_file_with_line_range as
line_tag or line_tags to prevent editing stale content.

Best for non-code files (configs, docs) needing precise line
control.

Examples:
  read_file_with_line_range(path="file.txt")
  read_file_with_line_range(path="file.txt", start_line=3, end_line=3)
  read_file_with_line_range(path="file.txt", start_line=10, end_line=20)
  read_file_with_line_range(path="file.txt", start_line=50)
