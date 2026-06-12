# mcp_client

Switch MCP target between local and cloud.

**local** (default): supports all 19 tools (read, interact, forms, JS scripting).
**cloud**: supports goto / markdown / links only (read-only). Use for sites
that need a proxy (e.g. Google, Wikimedia, blocked sites).

Switching is stateless and cleanly reversible — no lingering side effects.
