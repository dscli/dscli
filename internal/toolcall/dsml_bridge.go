package toolcall

import (
	"context"

	"github.com/dscli/dscli/internal/prompt"
)

// This file is the execution-kernel bridge for the internal/dsml package.
// Parsing and judgement live in dsml; the registry, role allow-set, and
// batch executor stay here so the dsml package imports toolcall (never the
// reverse). The wrappers below expose only what dsml needs and nothing
// DSML-shaped.

// RoleToolsSpec returns the role's tools spec ("all", "", or "a,b") from
// role_configs, falling back to roles.DefaultFor. Shared with GetAllTools so
// the web model can only ever call what the role is configured for.
func RoleToolsSpec(ctx context.Context, role string) string {
	return roleToolsSpec(ctx, role)
}

// RoleToolAllowSet converts the role's tools spec into an allow-set:
// nil = everything ("all"), empty map = nothing, non-empty map = those names.
func RoleToolAllowSet(ctx context.Context, role string) map[string]bool {
	return roleToolAllowSet(ctx, role)
}

// AllowSetFromSpec converts a stored spec ("all", "", "a,b") into an
// allow-set, the same meaning as RoleToolAllowSet without a role lookup.
func AllowSetFromSpec(spec string) map[string]bool {
	return allowSetFromSpec(spec)
}

// ExecuteToolCallsNoSave runs a batch of tool calls through the shared
// execution kernel WITHOUT persisting the resulting tool messages. The DSML
// web-chat path must not write into the current session's messages table:
// those extra tool messages would break the assistant<->tool pairing that
// CleanupReverse relies on, dropping the whole ask_expert turn from history.
func ExecuteToolCallsNoSave(ctx context.Context, tcs []prompt.ToolCall) (outcomes []ToolCallOutcome, dualUsers []prompt.Message) {
	return executeToolCalls(ctx, tcs, false)
}

// UnregisterTool removes a tool and its aliases from the in-memory registry.
// It exists so the dsml package's tests can clean up after themselves without
// reaching into toolRegistry, and so external tool management can remove a
// tool at runtime.
func UnregisterTool(name string) {
	toolRegistryRWMutex.Lock()
	defer toolRegistryRWMutex.Unlock()
	delete(toolRegistry, name)
	for alias, canonical := range toolAliases {
		if canonical == name {
			delete(toolAliases, alias)
		}
	}
}
