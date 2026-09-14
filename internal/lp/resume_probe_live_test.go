package lp

// Gated live probe for the resume read (webchatReadTail / ReadTail): it
// navigates to a real conversation and dumps what the server-truth read
// returns - message length, status, tail-role and, on failure, the refusal
// reason. READ-ONLY: it never sends a message and never executes a block;
// the tab is closed when the probe returns.
//
//	DSCLI_LIVE_RESUME_PROBE='<conversation id or URL>' \
//	  go test -v -timeout 300s -run TestLiveResumeProbe ./internal/lp/
//
// Requires Chrome plus a logged-in dscli profile; it opens a visible browser
// window. Without DSCLI_LIVE_RESUME_PROBE set it skips (the default in CI
// and in a normal test pass).

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestLiveResumeProbe(t *testing.T) {
	keep := strings.TrimSpace(os.Getenv("DSCLI_LIVE_RESUME_PROBE"))
	if keep == "" {
		t.Skip("set DSCLI_LIVE_RESUME_PROBE=<conversation id or URL> to run")
	}
	url := keep
	if ConversationIDFromURL(url) == "" {
		url = constructConversationURL(keep)
	}
	sess := newWebChatSession()
	defer sess.Close()
	content, status, tailRole, reason, ok := sess.ReadTail(context.Background(), url)
	t.Logf("url=%s ok=%v status=%q tailRole=%q reason=%q", url, ok, status, tailRole, reason)
	t.Logf("content (%d runes):\n%s", countRunes(content), content)
	if !ok {
		t.Fatalf("read failed: %s", reason)
	}
	if tailRole == "" {
		t.Error("tailRole missing (the read must report the newest message's role)")
	}
}
