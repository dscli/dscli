package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTryLock_Local(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".dscli")

	lk1, ok, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock 1: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("tryLock 1: expected to acquire lock")
	}
	defer lk1.Close()

	// Verify lock file exists
	lockPath := filepath.Join(cfgDir, "locks", "dscli.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file not found: %v", err)
	}

	// Second lock on same project should fail
	_, ok2, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock 2: unexpected error: %v", err)
	}
	if ok2 {
		t.Fatal("tryLock 2: expected lock to be held")
	}

	// PID should match our process
	pid := PID(cfgDir)
	if pid != os.Getpid() {
		t.Errorf("PID: expected %d, got %d", os.Getpid(), pid)
	}

	lk1.Close()

	// After release, should succeed
	lk3, ok3, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock 3: unexpected error: %v", err)
	}
	if !ok3 {
		t.Fatal("tryLock 3: expected to acquire lock after release")
	}
	lk3.Close()
}

func TestTryLock_Global(t *testing.T) {
	cfgDir := t.TempDir()

	lk, ok, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock Global: unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("tryLock Global: expected to acquire lock")
	}
	defer lk.Close()

	_, ok2, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock Global 2: unexpected error: %v", err)
	}
	if ok2 {
		t.Fatal("tryLock Global 2: expected lock to be held")
	}

	lk.Close()

	lk3, ok3, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock Global 3: unexpected error: %v", err)
	}
	if !ok3 {
		t.Fatal("tryLock Global 3: expected to acquire lock after release")
	}
	lk3.Close()
}

func TestTryLock_DifferentProjects(t *testing.T) {
	cfgDir1 := filepath.Join(t.TempDir(), ".dscli")
	cfgDir2 := filepath.Join(t.TempDir(), ".dscli")

	lk1, ok1, err := tryLock(cfgDir1)
	if err != nil {
		t.Fatalf("tryLock project 1: %v", err)
	}
	if !ok1 {
		t.Fatal("tryLock project 1: expected to acquire")
	}
	defer lk1.Close()

	lk2, ok2, err := tryLock(cfgDir2)
	if err != nil {
		t.Fatalf("tryLock project 2: %v", err)
	}
	if !ok2 {
		t.Fatal("tryLock project 2: expected to acquire (different project)")
	}
	lk2.Close()
}

func TestClose(t *testing.T) {
	cfgDir := filepath.Join(t.TempDir(), ".dscli")

	lk, ok, err := tryLock(cfgDir)
	if err != nil {
		t.Fatalf("tryLock: %v", err)
	}
	if !ok {
		t.Fatal("expected to acquire")
	}

	if err := lk.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Double close should be safe
	if err := lk.Close(); err != nil {
		t.Fatalf("Double Close: %v", err)
	}
}
