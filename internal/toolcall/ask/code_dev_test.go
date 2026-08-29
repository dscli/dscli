package ask

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/shell"
	"github.com/dscli/dscli/internal/toolcall"
)

// TestCodeDevToolStructure tests the basic structure of the code_dev tool.
func TestCodeDevToolStructure(t *testing.T) {
	if codeDevTool.Name != "code_dev" {
		t.Errorf("Expected tool name 'code_dev', got '%s'", codeDevTool.Name)
	}
	if codeDevTool.DisplayName != "Code Developer" {
		t.Errorf("Expected display name 'Code Developer', got '%s'", codeDevTool.DisplayName)
	}

	// The description must carry the delivery contract keywords so the
	// architect (and any caller) understands what the tool guarantees.
	description := codeDevTool.Description
	for _, keyword := range []string{"task", "implement", "commit", "keep", "test"} {
		if !strings.Contains(description, keyword) {
			t.Errorf("Tool description missing required keyword: %s", keyword)
		}
	}
	// 60 min: implementation runs many DSML tool-call rounds (implement →
	// test → iterate → commit), heavier than a review pass (30 min).
	if codeDevTool.Timeout != 60*time.Minute {
		t.Errorf("Expected timeout 60 minutes, got %v", codeDevTool.Timeout)
	}
	if codeDevTool.Category != "check" {
		t.Errorf("Expected category 'check', got '%s'", codeDevTool.Category)
	}
	// task and keep are conditional: express with anyOf (mirrors
	// quality_assurance), so a schema-validating caller knows one is enough.
	if _, ok := codeDevTool.Parameters["anyOf"]; !ok {
		t.Error("Expected anyOf in tool parameters (task or keep required)")
	}
}

// TestHandleCodeDevMissingInput verifies the handler refuses a call with
// neither task nor keep — the anyOf guard's defensive twin.
func TestHandleCodeDevMissingInput(t *testing.T) {
	_, _, err := handleCodeDev(context.Background(), toolcall.ToolArgs{})
	if err == nil {
		t.Fatal("expected error when neither task nor keep is provided")
	}
}

// TestHandleCodeDevWithTask verifies the normal path: task is sent to the
// developer role (mocked in test mode), and the mock reply is returned.
func TestHandleCodeDevWithTask(t *testing.T) {
	args := toolcall.ToolArgs{"task": "Add a cmd/hello.go command that prints hello."}
	result, _, err := handleCodeDev(context.Background(), args)
	if err != nil {
		t.Fatalf("handleCodeDev returned error: %v", err)
	}
	if !strings.Contains(result, "[MOCK]") {
		t.Fatalf("expected [MOCK] in result, got: %s", result)
	}
}

// TestHandleCodeDevKeepOnly verifies the resume path: keep without task
// resumes the interrupted developer conversation instead of starting a new
// one (mock devResumeFunc).
func TestHandleCodeDevKeepOnly(t *testing.T) {
	orig := devResumeFunc
	devResumeFunc = func(_ context.Context, opts lp.WebChatOptions) (lp.WebChatResult, error) {
		if opts.Role != "dev" {
			t.Errorf("expected role dev, got %q", opts.Role)
		}
		if opts.Keep != "conv-123" {
			t.Errorf("expected keep conv-123, got %q", opts.Keep)
		}
		return lp.WebChatResult{Content: "[MOCK-RESUME] developer report", Printed: true}, nil
	}
	defer func() { devResumeFunc = orig }()

	result, _, err := handleCodeDev(context.Background(), toolcall.ToolArgs{"keep": "conv-123"})
	if err != nil {
		t.Fatalf("handleCodeDev keep-only returned error: %v", err)
	}
	if !strings.Contains(result, "[MOCK-RESUME]") {
		t.Fatalf("expected [MOCK-RESUME] in result, got: %s", result)
	}
}

// TestBuildCodeDevRequest tests the pure request builder: task plus the
// delivery contract that steers the developer (implement → test → commit).
func TestBuildCodeDevRequest(t *testing.T) {
	req := buildCodeDevRequest("Implement X")
	for _, section := range []string{"## Implementation Task", "## Delivery Contract", "Commit ALL changes", "test suite"} {
		if !strings.Contains(req, section) {
			t.Errorf("Expected section %q in request, got:\n%s", section, req)
		}
	}
	if !strings.Contains(req, "Implement X") {
		t.Errorf("Expected task content preserved in request")
	}
}

// TestTruncateCodeDevRequest verifies oversized tasks are hard-truncated at
// the rune limit and the caller gets a warning.
func TestTruncateCodeDevRequest(t *testing.T) {
	big := strings.Repeat("x", maxUserInputLen+10000)
	req, warning := truncateCodeDevRequest(big)
	if countRunes(req) > maxUserInputLen {
		t.Errorf("truncated request still exceeds limit: %d > %d", countRunes(req), maxUserInputLen)
	}
	if !strings.Contains(req, "任务输入截断") {
		t.Error("expected truncation note in request")
	}
	if warning == "" {
		t.Error("expected a truncation warning")
	}

	// Small tasks pass through unchanged with no warning.
	req, warning = truncateCodeDevRequest("short task")
	if warning != "" {
		t.Errorf("unexpected warning for small task: %s", warning)
	}
	if !strings.Contains(req, "short task") {
		t.Error("small task content not preserved")
	}
}

// TestCheckWorkingTree covers the git-status interpretation: grep -v '^??'
// exits 1 when there are no tracked changes (clean tree or untracked only),
// and that must NOT be treated as a check failure (regression: the clean
// tree used to print "无法检查 git 状态").
func TestCheckWorkingTree(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "test@example.com")
	mustGit(t, dir, "config", "user.name", "tester")
	if err := os.WriteFile(dir+"/a.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "a.txt")
	mustGit(t, dir, "commit", "-m", "init")

	t.Run("clean", func(t *testing.T) {
		t.Chdir(dir)
		dirty, note := checkWorkingTree(context.Background())
		if dirty {
			t.Errorf("clean tree reported dirty: %s", note)
		}
		if !strings.Contains(note, "干净") {
			t.Errorf("clean tree note = %q, want 干净", note)
		}
	})

	t.Run("untracked only", func(t *testing.T) {
		t.Chdir(dir)
		if err := os.WriteFile(dir+"/b.txt", []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}
		dirty, _ := checkWorkingTree(context.Background())
		if dirty {
			t.Error("untracked-only tree reported dirty (should be ignored)")
		}
	})

	t.Run("tracked modified", func(t *testing.T) {
		t.Chdir(dir)
		if err := os.WriteFile(dir+"/a.txt", []byte("changed"), 0o644); err != nil {
			t.Fatal(err)
		}
		dirty, note := checkWorkingTree(context.Background())
		if !dirty {
			t.Errorf("modified tree reported clean: %s", note)
		}
	})

	t.Run("not a git repository", func(t *testing.T) {
		t.Chdir(t.TempDir())
		dirty, _ := checkWorkingTree(context.Background())
		if dirty {
			t.Error("non-git directory reported dirty")
		}
	})
}

// mustGit runs a git command in dir and fails the test on error.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	script := "git -C " + dir + " " + strings.Join(args, " ")
	if _, err := shell.SimpleExecute(context.Background(), script); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}
