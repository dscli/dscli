// Package mcphub manages multiple MCP server connections and provides
// unified tool discovery and dispatch for the dscli tool framework.
package mcphub

import "context"

// Init initializes the global hub with built-in and user-configured servers.
// Called during startup from the web tool package init.
func Init(ctx context.Context) error {
	return globalHub.doInit(ctx)
}

// Dispatch routes a tool call to the correct MCP server based on the
// tool name prefix. The tool name format is "serverName_toolName".
func Dispatch(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	return globalHub.doDispatch(ctx, toolName, argsRaw)
}
