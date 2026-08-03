# mcp_client

Switch an MCP server's transport between local and cloud.

**local** (default): stdio transport (subprocess).
**cloud**: SSE transport. Use for sites that need a proxy
(e.g. Google, Wikimedia, blocked sites).

The `server` parameter selects which MCP server to switch (required, e.g.
`code`). Configure local/cloud variants in your `mcp-servers` config block.
There are no built-in MCP servers - every server is configured explicitly.

Switching affects the process-level singleton — the new target persists
for the lifetime of the dscli process (all future sessions in the same process).
