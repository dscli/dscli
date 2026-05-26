# sql

Execute read-only SQL queries against the dscli SQLite database.

Supports sqlite3 CLI dot-commands (auto-translated to SQL):

- `.schema` → list all table schemas
- `.schema <name>` → schema of a specific table
- `.tables` → list all tables
- `.indices <table>` → list indices on a table
Query is checked for safety:

- SELECT only (no INSERT/UPDATE/DELETE/DROP/ALTER/CREATE)
- EXPLAIN QUERY PLAN is allowed
- Read-only PRAGMA (table_info, index_list, etc.) are allowed
Results are formatted as a table with aligned columns, truncated at 50 rows.

Examples:

  1. Explore schema: `.tables`
  2. View table structure: `.schema memories`
  3. Query with filter: `SELECT name, type FROM memories WHERE type = 'decision' LIMIT 5`
  4. Count rows: `SELECT COUNT(*) FROM memories`
  5. Search FTS: `SELECT highlight(memories_fts, 0, '<b>', '</b>') FROM memories_fts WHERE memories_fts MATCH 'search'`

Timeout: default 30s. Set `timeout` (seconds) to override.
