Static analysis check

Run static checks on files, directories, or packages.
Supports Go (staticcheck) and Python (ruff). Three modes:
file, directory, recursive (...).

Returns file:line:col: message diagnostics on issues, or
success summary when clean.

Examples:
  flycheck(path="internal/flycheck/flycheck.go")
  flycheck(path="internal/toolcall/")
  flycheck(path="internal/...")
