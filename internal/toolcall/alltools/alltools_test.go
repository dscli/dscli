package alltools

import (
	"context"
	"testing"

	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/toolcall"
)

func TestGetAllTools(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		checker func(toolcall.Tool) bool
	}{
		// Strict tools must carry a closed schema (additionalProperties=false);
		// lenient tools (e.g. MCP servers with open schemas) are exempt —
		// OpenAI-compatible APIs reject strict=true with additionalProperties
		// that is not literally false.
		{"strict implies closed schema", func(tool toolcall.Tool) bool {
			if !tool.Function.Strict {
				return true // lenient tool — no strict-mode requirements
			}
			ap, ok := tool.Function.Parameters["additionalProperties"]
			if !ok {
				return false
			}
			b, ok := ap.(bool)
			return ok && !b
		}},
		{"not too large", func(tool toolcall.Tool) bool {
			return tool.GetTokens() <= 1600 // around 1.5K
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAllTools(context.Background())
			for _, tool := range got {
				if !tt.checker(tool) {
					t.Fatal(tool.Function.Strict, tool.Function.Name, tool.Function.Description)
				}
			}
		})
	}
}

// TestDevDefaultToolsAllRegistered guards the explicit DevDefaultTools list:
// every name must exist in the tool registry, so a single typo cannot
// silently drop a development tool from the dev role.
func TestDevDefaultToolsAllRegistered(t *testing.T) {
	known := make(map[string]bool, len(toolcall.KnownToolNames()))
	for _, n := range toolcall.KnownToolNames() {
		known[n] = true
	}
	for _, n := range roles.ParseToolsList(roles.DevDefaultTools) {
		if !known[n] {
			t.Errorf("DevDefaultTools entry %q is not a registered tool", n)
		}
	}
}
