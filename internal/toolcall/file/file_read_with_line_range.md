Read file line range, awk-compatible output.

Read specific lines from a file. Output format matches:
  awk 'NR>=start && NR<=end {print NR": "$0}'

Best for non-code files (configs, docs) needing precise line
control.

Examples:
  read_file_with_line_range(path="file.txt")
  read_file_with_line_range(path="file.txt", start_line=3, end_line=3)
  read_file_with_line_range(path="file.txt", start_line=10, end_line=20)
  read_file_with_line_range(path="file.txt", start_line=50)
