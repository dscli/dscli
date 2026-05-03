package prompt

import (
	"os"
	"testing"

	"gitcode.com/dscli/dscli/internal/tokenizer"
)

// TestMain pre-warms the GSE tokenizer so per-test timings are accurate.
// Dictionary loading (~1.4s) is attributed to setup, not TestSearchMessages.
func TestMain(m *testing.M) {
	_ = tokenizer.SanitizeFTS("warmup")
	os.Exit(m.Run())
}
