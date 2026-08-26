package lp

import (
	"os"
	"strings"
	"testing"
)

// TestChromiumEnvClearBlank checks that chromiumEnvClear() returns blank
// assignments ("KEY=") for both lower- and upper-case proxy variables, so
// that appending them after os.Environ() makes the last value win for
// every duplicate key (Go's os/exec keeps only the last value of
// duplicated env keys).
func TestChromiumEnvClearBlank(t *testing.T) {
	clear := chromiumEnvClear()
	if len(clear) == 0 {
		t.Fatal("chromiumEnvClear() returned no entries")
	}
	seen := map[string]bool{}
	for _, entry := range clear {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			t.Errorf("entry %q is not a KEY= assignment", entry)
			continue
		}
		if value != "" {
			t.Errorf("entry %q must blank the variable, got value %q", entry, value)
		}
		seen[key] = true
	}
	for _, key := range []string{
		"http_proxy", "https_proxy", "all_proxy",
		"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY",
	} {
		if !seen[key] {
			t.Errorf("chromiumEnvClear() missing %s", key)
		}
	}
}

// TestChromiumEnvClearOverridesParent verifies the merge semantics relied
// upon: after appending the clear entries to os.Environ(), the effective
// proxy values (last occurrence wins) are empty, mirroring what chromedp
// does at launch (os.Environ() + initEnv) and what Go's os/exec dedups.
func TestChromiumEnvClearOverridesParent(t *testing.T) {
	t.Setenv("http_proxy", "socks5h://localhost:8777")
	t.Setenv("https_proxy", "socks5h://localhost:8777")
	t.Setenv("HTTPS_PROXY", "socks5h://localhost:8777")

	merged := append(os.Environ(), chromiumEnvClear()...)

	// Simulate Go's dedupEnv: keep the last value per key.
	last := map[string]string{}
	for _, entry := range merged {
		key, value, _ := strings.Cut(entry, "=")
		last[key] = value
	}
	for _, key := range []string{"http_proxy", "https_proxy", "HTTPS_PROXY"} {
		if last[key] != "" {
			t.Errorf("after merge, %s = %q, want empty", key, last[key])
		}
	}
}
