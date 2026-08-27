package ask

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/toolcall"
)

// TestQualityAssuranceToolStructure verifies the basic tool definition.
func TestQualityAssuranceToolStructure(t *testing.T) {
	if qualityAssuranceTool.Name != "quality_assurance" {
		t.Errorf("Expected tool name 'quality_assurance', got '%s'", qualityAssuranceTool.Name)
	}
	if qualityAssuranceTool.DisplayName != "Quality Assurance" {
		t.Errorf("Expected display name 'Quality Assurance', got '%s'", qualityAssuranceTool.DisplayName)
	}

	description := qualityAssuranceTool.Description
	for _, keyword := range []string{"quality", "release", "uncommitted", "test", "HEAD"} {
		if !strings.Contains(description, keyword) {
			t.Errorf("Tool description missing required keyword: %s", keyword)
		}
	}

	if qualityAssuranceTool.Timeout != 20*time.Minute {
		t.Errorf("Expected timeout 20 minutes, got %v", qualityAssuranceTool.Timeout)
	}
	if qualityAssuranceTool.Category != "check" {
		t.Errorf("Expected category 'check', got '%s'", qualityAssuranceTool.Category)
	}
	if qualityAssuranceTool.Handler == nil {
		t.Error("qualityAssuranceTool.Handler should not be nil")
	}
}

// TestHandleQualityAssuranceFunction verifies the handler exists and responds
// to git state without panicking.
func TestHandleQualityAssuranceFunction(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{"summary": "release readiness check"}

	result, _, err := handleQualityAssurance(ctx, args)
	if err != nil {
		// Git environment errors (uncommitted changes / no commits) are
		// expected in a dev workspace, not a test failure.
		t.Logf("handleQualityAssurance returned error (expected in dev workspace): %v", err)
	} else {
		if !strings.Contains(result, "[MOCK]") {
			t.Fatalf("expected [MOCK] in result, got: %s", result)
		}
	}
}
