Find code definitions by name with type filter

Searches code files for definitions by name with optional
type filter. More precise than text search — understands
code structure.

Examples:
  search_code_definition(path="user.go", pattern="user")
  search_code_definition(path="main.go", pattern="handle", type_filter="function")
  search_code_definition(path="service.go", pattern="", type_filter="method")
