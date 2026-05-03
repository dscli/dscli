package tokenizer

import (
	"os"
	"testing"
)

// TestMain pre-warms the GSE tokenizer so per-test timings are accurate.
// Dictionary loading (~1.4s) is attributed to setup, not TestSanitizeFTS.
func TestMain(m *testing.M) {
	_ = Tokenize("warmup")
	os.Exit(m.Run())
}
