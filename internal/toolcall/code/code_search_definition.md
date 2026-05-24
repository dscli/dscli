# code_search_definition

Find code definitions by name with type filter.

Searches code files for definitions by name with optional
type filter. More precise than text search — understands
code structure. Supports single file or directory (flat, non-recursive).

Examples:
  search_code_definition(path="user.go", pattern="user")
  search_code_definition(path="main.go", pattern="handle", type_filter="function")
  search_code_definition(path="service.go", pattern="", type_filter="method")
  search_code_definition(path=".", pattern="Search")
  search_code_definition(path="./internal", pattern="handle", type_filter="function")
