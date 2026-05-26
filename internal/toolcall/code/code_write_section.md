# write_code_section

Write code section by semantic selector.

Modify specific code sections using semantic selectors:
  function:name, class:name, method:Type.Method, lines:start-end.

context (default true): after editing, returns a context
window showing the file state around the edit. Set false
to suppress and save output tokens.

Examples:
  write_code_section(path="main.go", selector="function:main", new_content="func main() {...}")
  write_code_section(path="user.go", selector="method:User.GetName", new_content="...")
