Read code section by semantic selector

Read specific code sections using semantic selectors instead
of line numbers.

Selectors:
  function:name  — read a function
  class:name     — read a class/struct
  method:Type.Method — read a method
  lines:start-end — read line range (fallback)

Smarter than line-based tools — auto-locates code by structure.

Examples:
  read_code_section(path="main.go", selector="function:main")
  read_code_section(path="user.go", selector="method:User.GetName")
  read_code_section(path="config.yaml", selector="lines:10-20")
