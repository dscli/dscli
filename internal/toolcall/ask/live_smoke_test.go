package ask

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// TestLiveCodeReviewSmoke is a manual, network-dependent smoke test: it runs
// the real code_review handler against the last commit and the live DeepSeek
// web chat (browser + attachment uploads). It is skipped unless
// DSCLI_LIVE_SMOKE=1 is set, so the normal suite and CI never touch the
// network:
//
//	DSCLI_LIVE_SMOKE=1 go test -run TestLiveCodeReviewSmoke -timeout 900s ./internal/toolcall/ask/
func TestLiveCodeReviewSmoke(t *testing.T) {
	if os.Getenv("DSCLI_LIVE_SMOKE") != "1" {
		t.Skip("live smoke: set DSCLI_LIVE_SMOKE=1 (needs Chrome login and network)")
	}
	orig := askExpertWithRoleFunc
	askExpertWithRoleFunc = askExpertWebChat // bypass the test-mode [MOCK]
	t.Cleanup(func() { askExpertWithRoleFunc = orig })

	result, warning, err := handleCodeReview(context.Background(), toolcall.ToolArgs{
		"summary": "Smoke test of the attachment-based review flow: confirm in one short paragraph which attachments you received (guide, patch, source files) and whether you can follow the guide; no full review needed.",
		"since":   "-1",
	})
	t.Logf("warning: %s", warning)
	if err != nil {
		t.Fatalf("handleCodeReview: %v", err)
	}
	if len(strings.TrimSpace(result)) < 40 {
		t.Errorf("unexpectedly short result: %q", result)
	}
	t.Logf("RESULT:\n%s", result)
}
