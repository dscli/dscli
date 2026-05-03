package memories

import (
	"os"
	"testing"

	"gitcode.com/dscli/dscli/internal/tokenizer"
)

// TestMain pre-warms the GSE tokenizer so per-test timings are accurate.
// Dictionary loading (~1.4s) is attributed to setup, not TestInsertAndDeleteFTS.
func TestMain(m *testing.M) {
	_ = tokenizer.Tokenize("warmup")
	os.Exit(m.Run())
}
