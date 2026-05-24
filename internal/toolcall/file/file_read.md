Read file content with line numbers.

Output format matches:
  awk 'NR>=1 {print NR": "$0}'

By default (tags=true), each line includes a 4-character
checksum tag for CAS (check-and-set) safety:
  10:Q8fA int count = 10;

Set tags=false to omit tags.

Equivalent to read_file_with_line_range without line range
parameters.
