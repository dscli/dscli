# code_read_structure

Read code file structure (functions/classes).

Returns a human-readable summary and complete JSON structure
for single files. For directories, aggregates structure of
all code files with per-file summaries.

Use before write_code_section or read_code_section to
understand file or package layout.

Examples:
  read_code_structure(path="main.go")
  read_code_structure(path="internal/config")
  read_code_structure(path="user.py")
