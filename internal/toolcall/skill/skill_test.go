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
		t.Run(name, func(t *testing.T) {
			tool, ok := toolcall.GetToolDef(ctx, name)
			if !ok {
				t.Fatalf("tool %q has not been registered; init() may not have been called", name)
			}
			if tool.Name != name {
				t.Errorf("Name = %q, want %q", tool.Name, name)
			}
			if tool.Handler == nil {
				t.Error("Handler is nil")
			}
			if tool.DisplayName == "" {
				t.Error("DisplayName is empty")
			}
			if tool.Description == "" {
				t.Error("Description is empty")
			}
			if tool.Category == "" {
				t.Error("Category is empty")
			}
		})
	}
}
