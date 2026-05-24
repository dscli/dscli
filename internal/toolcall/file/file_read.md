# read_file

Read file content with line numbers.

Output format matches:
  awk 'NR>=1 {print NR": "$0}'

Each line includes a 4-character checksum tag for CAS
(check-and-set) safety:
  10:Q8fA int count = 10;

Tags can be passed to write_file_with_line_range as
line_tag or line_tags to prevent editing stale content.

Equivalent to read_file_with_line_range without line range
parameters.
