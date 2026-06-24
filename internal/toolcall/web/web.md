# mcp_client

Switch MCP target between local and cloud.

**local** (default): supports all tools via stdio transport (e.g., lightpanda native MCP).
**cloud**: connects via SSE transport (e.g., LightPanda Cloud). Use for sites
that need a proxy (e.g. Google, Wikimedia, blocked sites).

The `server` parameter selects which MCP server to switch (default: `lightpanda`).
Configure cloud variants in your `mcp-servers` YAML config file.

Switching affects the process-level singleton — the new target persists
for the lifetime of the dscli process (all future sessions in the same process).
