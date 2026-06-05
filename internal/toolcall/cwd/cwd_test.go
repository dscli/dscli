package cwd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dscli/dscli/internal/context"
)

// saveState captures the current global state for restoration.
func saveState() (cwd string, projectRoot string, stack []dirEntry) {
	cwd, _ = os.Getwd()
	projectRoot = context.ProjectRoot
	dirStackMu.Lock()
	stack = make([]dirEntry, len(dirStack))
	copy(stack, dirStack)
	dirStackMu.Unlock()
	return
}

// restoreState restores global state to saved values.
func restoreState(cwd, projectRoot string, stack []dirEntry) {
	os.Chdir(cwd)
	context.ProjectRoot = projectRoot
	dirStackMu.Lock()
	dirStack = stack
	dirStackMu.Unlock()
}

func TestCWDGet(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	result, _, err := handleCWDGet(context.TODO(), ToolArgs{})
	if err != nil {
		t.Fatalf("cwd_get failed: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	t.Logf("cwd_get result:\n%s", result)
}

func TestCWDPushPopRoundtrip(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	tmpDir := t.TempDir()

	// Push
	result, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": tmpDir})
	if err != nil {
		t.Fatalf("cwd_push failed: %v", err)
	}
	t.Logf("push: %s", result)

	// Verify CWD changed
	cwd, _ := os.Getwd()
	if !samePath(cwd, tmpDir) {
		t.Errorf("expected CWD %q, got %q", tmpDir, cwd)
	}

	// Pop
	result, _, err = handleCWDPop(context.TODO(), ToolArgs{})
	if err != nil {
		t.Fatalf("cwd_pop failed: %v", err)
	}
	t.Logf("pop: %s", result)

	// Verify CWD restored
	cwd, _ = os.Getwd()
	if !samePath(cwd, origCWD) {
		t.Errorf("expected CWD restored to %q, got %q", origCWD, cwd)
	}

	// Verify ProjectRoot restored
	if context.ProjectRoot != origPR {
		t.Errorf("expected ProjectRoot %q, got %q", origPR, context.ProjectRoot)
	}
}

func TestCWDPushRelativePath(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	tmpDir := t.TempDir()
	// chdir to parent, then push with relative path
	if err := os.Chdir(filepath.Dir(tmpDir)); err != nil {
		t.Fatal(err)
	}
	base := filepath.Base(tmpDir)

	result, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": base})
	if err != nil {
		t.Fatalf("cwd_push with relative path failed: %v", err)
	}
	t.Logf("push relative: %s", result)

	cwd, _ := os.Getwd()
	if !samePath(cwd, tmpDir) {
		t.Errorf("expected CWD %q, got %q", tmpDir, cwd)
	}
}

func TestCWDPushSameDir(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	cwd, _ := os.Getwd()

	result, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": cwd})
	if err != nil {
		t.Fatalf("push to same dir should not error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result for same-dir push")
	}
	t.Logf("push same: %s", result)

	// Stack should be unchanged
	dirStackMu.Lock()
	depth := len(dirStack)
	dirStackMu.Unlock()
	if depth != 0 {
		t.Errorf("stack should be empty after same-dir push, got depth %d", depth)
	}
}

func TestCWDPushNonExistent(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	_, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": "/nonexistent/path/12345"})
	if err == nil {
		t.Error("expected error for non-existent path")
	}
	t.Logf("expected error: %v", err)
}

func TestCWDPushFileNotDir(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": tmpFile})
	if err == nil {
		t.Error("expected error for file path")
	}
	t.Logf("expected error: %v", err)
}

func TestCWDPopEmpty(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	// Clear the stack for this test
	dirStackMu.Lock()
	dirStack = nil
	dirStackMu.Unlock()

	result, _, err := handleCWDPop(context.TODO(), ToolArgs{})
	if err != nil {
		t.Errorf("pop on empty stack should not error: %v", err)
	}
	if result == "" {
		t.Error("expected message for empty stack")
	}
	t.Logf("pop empty: %s", result)
}

func TestCWDPushMultiple(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	d1 := t.TempDir()
	d2 := t.TempDir()

	// Push d1
	if _, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": d1}); err != nil {
		t.Fatal(err)
	}
	// Push d2
	if _, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": d2}); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	if !samePath(cwd, d2) {
		t.Errorf("expected CWD %q, got %q", d2, cwd)
	}

	// Pop to d1
	if _, _, err := handleCWDPop(context.TODO(), ToolArgs{}); err != nil {
		t.Fatal(err)
	}
	cwd, _ = os.Getwd()
	if !samePath(cwd, d1) {
		t.Errorf("expected CWD restored to %q, got %q", d1, cwd)
	}

	// Pop to original
	if _, _, err := handleCWDPop(context.TODO(), ToolArgs{}); err != nil {
		t.Fatal(err)
	}
	cwd, _ = os.Getwd()
	if !samePath(cwd, origCWD) {
		t.Errorf("expected CWD restored to %q, got %q", origCWD, cwd)
	}

	// Stack should be empty
	dirStackMu.Lock()
	depth := len(dirStack)
	dirStackMu.Unlock()
	if depth != 0 {
		t.Errorf("stack should be empty, got depth %d", depth)
	}
}

func TestCWDPopRestoresProjectRoot(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	tmpDir := t.TempDir()

	// Push to temp (non-git) dir; ProjectRoot will change
	_, _, err := handleCWDPush(context.TODO(), ToolArgs{"path": tmpDir})
	if err != nil {
		t.Fatal(err)
	}

	// ProjectRoot should now be the temp dir (non-git fallback)
	if context.ProjectRoot != tmpDir {
		// Could also be the resolved path
		resolved, _ := filepath.EvalSymlinks(tmpDir)
		if context.ProjectRoot != resolved {
			t.Errorf("expected ProjectRoot to be temp dir, got %q", context.ProjectRoot)
		}
	}

	// Pop should restore original ProjectRoot
	_, _, err = handleCWDPop(context.TODO(), ToolArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if context.ProjectRoot != origPR {
		t.Errorf("expected ProjectRoot restored to %q, got %q", origPR, context.ProjectRoot)
	}
}

func TestCWDGetShowsDepth(t *testing.T) {
	origCWD, origPR, origStack := saveState()
	t.Cleanup(func() { restoreState(origCWD, origPR, origStack) })

	// Depth should be 0 initially
	result, _, _ := handleCWDGet(context.TODO(), ToolArgs{})
	t.Logf("initial: %s", result)

	// Push
	tmpDir := t.TempDir()
	handleCWDPush(context.TODO(), ToolArgs{"path": tmpDir})

	result, _, _ = handleCWDGet(context.TODO(), ToolArgs{})
	t.Logf("after push: %s", result)

	// Pop
	handleCWDPop(context.TODO(), ToolArgs{})

	result, _, _ = handleCWDGet(context.TODO(), ToolArgs{})
	t.Logf("after pop: %s", result)
}

// samePath compares two paths after resolving symlinks.
func samePath(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}
