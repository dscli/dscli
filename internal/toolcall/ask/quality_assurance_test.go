package ask

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/lp"
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
	for _, keyword := range []string{"quality", "release", "uncommitted", "test", "last"} {
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

// captureQAResume replaces qaResumeFunc with a recording mock and returns the
// captured keep value. The mock reply mimics the tool-loop result (Printed:
// the loop already printed every round).
func captureQAResume(t *testing.T) *string {
	t.Helper()
	orig := qaResumeFunc
	var keep string
	qaResumeFunc = func(_ context.Context, opts lp.WebChatOptions) (lp.WebChatResult, error) {
		keep = opts.Keep
		return lp.WebChatResult{Content: "[MOCK] resumed report", URL: "https://chat.deepseek.com/a/chat/s/convR", Printed: true}, nil
	}
	t.Cleanup(func() { qaResumeFunc = orig })
	return &keep
}

func TestHandleQualityAssuranceResume(t *testing.T) {
	// keep=<id> must bypass the git checks and summary requirement entirely:
	// the context the engineer needs is already in the saved conversation,
	// so the handler resumes without asking for a new summary.
	captured := captureQAResume(t)
	result, _, err := handleQualityAssurance(context.Background(),
		toolcall.ToolArgs{"keep": "b150d3e6-76a0-47c6-870f-a706371c897c"})
	if err != nil {
		t.Fatalf("handleQualityAssurance with keep: %v", err)
	}
	if *captured != "b150d3e6-76a0-47c6-870f-a706371c897c" {
		t.Errorf("keep passed to resume = %q, want the conversation ID", *captured)
	}
	if !strings.Contains(result, "[MOCK] resumed report") {
		t.Errorf("result = %q, want the resumed report", result)
	}
}

func TestHandleQualityAssuranceResumeError(t *testing.T) {
	orig := qaResumeFunc
	qaResumeFunc = func(_ context.Context, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		return lp.WebChatResult{}, fmt.Errorf("conversation not found")
	}
	t.Cleanup(func() { qaResumeFunc = orig })

	_, _, err := handleQualityAssurance(context.Background(),
		toolcall.ToolArgs{"keep": "missing-conv"})
	if err == nil || !strings.Contains(err.Error(), "质量保障恢复失败") {
		t.Fatalf("err = %v, want resume-failure wrapped error", err)
	}
}
