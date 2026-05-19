package userservice

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ---- pidfile helpers ----

func TestWriteReadRemovePid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	const name = "test-svc"
	const pid = 12345

	if err := writePid(name, pid); err != nil {
		t.Fatalf("writePid: %v", err)
	}

	got, err := readPid(name)
	if err != nil {
		t.Fatalf("readPid: %v", err)
	}
	if got != pid {
		t.Errorf("readPid = %d, want %d", got, pid)
	}

	pp, err := pidPath(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(pp); err != nil {
		t.Errorf("pidfile should exist: %v", err)
	}

	if err := removePidFile(name); err != nil {
		t.Fatalf("removePidFile: %v", err)
	}
	if _, err := os.Stat(pp); !os.IsNotExist(err) {
		t.Error("pidfile should be removed")
	}
}

func TestReadPidMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	_, err := readPid("nonexistent")
	if err == nil {
		t.Error("readPid should fail for missing pidfile")
	}
}

func TestRemovePidFileMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := removePidFile("nonexistent"); err != nil {
		t.Errorf("removePidFile should be idempotent: %v", err)
	}
}

// ---- serviceConfig helpers ----

func TestSaveLoadServiceConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	const name, desc, execStart = "test-svc", "Test Service", "/usr/bin/test"
	args := []string{"/usr/bin/test", "--flag", "--opt=val"}

	if err := saveServiceConfig(name, desc, execStart, args); err != nil {
		t.Fatalf("saveServiceConfig: %v", err)
	}

	cfg, err := loadServiceConfig(name)
	if err != nil {
		t.Fatalf("loadServiceConfig: %v", err)
	}
	if cfg.Name != name {
		t.Errorf("Name = %q, want %q", cfg.Name, name)
	}
	if cfg.Desc != desc {
		t.Errorf("Desc = %q, want %q", cfg.Desc, desc)
	}
	if cfg.ExecStart != execStart {
		t.Errorf("ExecStart = %q, want %q", cfg.ExecStart, execStart)
	}
	if len(cfg.Args) != len(args) {
		t.Fatalf("Args len = %d, want %d", len(cfg.Args), len(args))
	}
	for i, a := range args {
		if cfg.Args[i] != a {
			t.Errorf("Args[%d] = %q, want %q", i, cfg.Args[i], a)
		}
	}
}

// ---- buildCmd ----

func TestBuildCmd(t *testing.T) {
	tests := []struct {
		name string
		cfg  *serviceConfig
		want string
	}{
		{
			name: "with Args",
			cfg: &serviceConfig{
				Name: "test",
				Args: []string{"/usr/bin/echo", "hello", "world"},
			},
			want: "/usr/bin/echo hello world",
		},
		{
			name: "fallback ExecStart",
			cfg: &serviceConfig{
				Name:      "test",
				ExecStart: "/usr/bin/echo hello world",
			},
			want: "/usr/bin/echo hello world",
		},
		{
			name: "Args takes precedence",
			cfg: &serviceConfig{
				Name:      "test",
				ExecStart: "/usr/bin/wrong",
				Args:      []string{"/usr/bin/correct"},
			},
			want: "/usr/bin/correct",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := buildCmd(tt.cfg)
			if err != nil {
				t.Fatalf("buildCmd: %v", err)
			}
			if cmd.String() != tt.want {
				t.Errorf("cmd.String() = %q, want %q", cmd.String(), tt.want)
			}
		})
	}
}

func TestBuildCmdEmpty(t *testing.T) {
	cfg := &serviceConfig{Name: "test"}
	_, err := buildCmd(cfg)
	if err == nil {
		t.Error("buildCmd should fail with empty ExecStart and no Args")
	}
}

// ---- fallback methods (safe for unit test) ----

func TestFallbackIsRunningNoPidFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	f := fallback{}
	if f.isRunning("nonexistent") {
		t.Error("isRunning should return false when no pidfile")
	}
}

func TestFallbackStopNoPidFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	f := fallback{}
	if err := f.stop("nonexistent"); err != nil {
		t.Errorf("stop should be idempotent: %v", err)
	}
}

func TestFallbackDelete(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	f := fallback{}
	if err := f.delete("nonexistent"); err != nil {
		t.Errorf("delete should be idempotent: %v", err)
	}

	if err := writePid("stale", 99999); err != nil {
		t.Fatal(err)
	}
	if err := f.delete("stale"); err != nil {
		t.Errorf("delete with stale pid: %v", err)
	}
	pp, _ := pidPath("stale")
	if _, err := os.Stat(pp); !os.IsNotExist(err) {
		t.Error("pidfile should be removed after delete")
	}
}

func TestFallbackIsRunningStalePid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	if err := writePid("stale", 99999); err != nil {
		t.Fatal(err)
	}

	f := fallback{}
	if f.isRunning("stale") {
		t.Error("isRunning should return false for stale pid")
	}

	// Stale pidfile should be cleaned up.
	pp, _ := pidPath("stale")
	if _, err := os.Stat(pp); !os.IsNotExist(err) {
		t.Error("stale pidfile should be removed by isRunning")
	}
}

// ---- config round-trip via serviceDir ----

func TestConfigPathRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	sd, err := serviceDir()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(tmp, ".dscli", "services")
	if sd != expected {
		t.Errorf("serviceDir = %q, want %q", sd, expected)
	}

	cp, err := serviceConfigPath("foo")
	if err != nil {
		t.Fatal(err)
	}
	if cp != filepath.Join(sd, "foo.json") {
		t.Errorf("serviceConfigPath = %q, want %q", cp, filepath.Join(sd, "foo.json"))
	}
}

// ---- exec.Cmd args preservation in Create ----

func TestCreatePreservesArgs(t *testing.T) {
	// This test verifies that args are stored in the JSON config.
	// When systemd is available, Create also writes a systemd unit, which
	// can interfere with the real systemd user instance. Skip in that case
	// — the "Args persisted" invariant is still verified by
	// TestSaveLoadServiceConfig.
	if systemdUserAvailable() {
		t.Skip("systemd available: Create interacts with real systemd, skip")
	}

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	cmd := exec.Command("echo", "hello", "world")
	err := Create("test-echo", "Echo Test", cmd)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	cfg, err := loadServiceConfig("test-echo")
	if err != nil {
		t.Fatalf("loadServiceConfig: %v", err)
	}
	if len(cfg.Args) < 2 {
		t.Fatalf("expected at least 2 args, got %v", cfg.Args)
	}
	if cfg.Args[1] != "hello" || cfg.Args[2] != "world" {
		t.Errorf("Args = %v, want [..., hello, world]", cfg.Args)
	}
}
