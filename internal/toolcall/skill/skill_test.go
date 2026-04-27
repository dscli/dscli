package skill

import (
	"context"
	"testing"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

// TestRegistration verifies both skill tools are registered via init().
// This is a smoke test to catch the exact type of bug this package was created to fix:
// if the package is not imported, init() never runs and tools won't be registered.
func TestRegistration(t *testing.T) {
	ctx := context.Background()

	for _, name := range []string{"skill_by_name", "skill_search"} {
		tool, ok := toolcall.GetToolDef(ctx, name)
		if !ok {
			t.Errorf("tool %q not registered — ensure package is imported in alltools", name)
			continue
		}
		if tool.Name != name {
			t.Errorf("tool %q has unexpected Name field: %q", name, tool.Name)
		}
		if tool.Handler == nil {
			t.Errorf("tool %q has nil Handler", name)
		}
	}
}
