package userservice

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// fallback implements start/stop/delete/isRunning via direct process
// management (pidfile + daemonized child).  It is used when no native
// service manager is available — systemd on Linux, launchd on macOS, or
// any manager on other platforms.
type fallback struct{}

// start daemonizes the command stored in the service config and records
// the child PID in ~/.dscli/services/<name>.pid.
//
// It is idempotent: if a process with the recorded PID is already alive,
// start returns nil without launching a duplicate.
func (f fallback) start(name string) error {
	if f.isRunning(name) {
		return nil
	}

	cfg, err := loadServiceConfig(name)
	if err != nil {
		return err
	}

	// Clean stale pidfile before launching.
	if pp, err := pidPath(name); err == nil {
		_ = os.Remove(pp)
	}

	cmd, err := buildCmd(cfg)
	if err != nil {
		return err
	}

	// Daemonize — Setsid on Unix, CREATE_NEW_PROCESS_GROUP on Windows.
	daemonizeCmd(cmd)

	// Redirect stdio to the null device so the child does not block
	// waiting for a reader/writer.
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("userservice: open %s: %w", os.DevNull, err)
	}
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("userservice: start %s: %w", name, err)
	}

	return writePid(name, cmd.Process.Pid)
}

// stop sends SIGTERM (or taskkill on Windows) to the process recorded in
// the pidfile, then removes the pidfile.
//
// It is idempotent: a missing pidfile or a process that is already dead
// is not an error.
func (f fallback) stop(name string) error {
	pid, err := readPid(name)
	if err != nil {
		return nil
	}

	if !isProcessAlive(pid) {
		_ = removePidFile(name)
		return nil
	}

	if err := killProcess(pid); err != nil {
		return fmt.Errorf("userservice: stop %s (pid %d): %w", name, pid, err)
	}

	_ = removePidFile(name)
	return nil
}

// delete stops the process and removes the pidfile.  The JSON registry
// entry is removed by the public Delete function.
func (f fallback) delete(name string) error {
	_ = f.stop(name)
	_ = removePidFile(name)
	return nil
}

// isRunning reports whether a process recorded in the pidfile is alive.
func (f fallback) isRunning(name string) bool {
	pid, err := readPid(name)
	if err != nil {
		return false
	}
	if !isProcessAlive(pid) {
		_ = removePidFile(name)
		return false
	}
	return true
}

// ---- PID file helpers ----

// pidPath returns the path to the pidfile for name.
func pidPath(name string) (string, error) {
	sd, err := serviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(sd, name+".pid"), nil
}

// readPid reads the pidfile for name and returns the recorded PID.
func readPid(name string) (int, error) {
	pp, err := pidPath(name)
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(pp)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// writePid writes pid to the pidfile for name.
// writePid writes pid to the pidfile for name.
// Creates the services directory if it does not exist.
func writePid(name string, pid int) error {
	pp, err := pidPath(name)
	if err != nil {
		return err
	}
	// Ensure the services directory exists.
	sd, err := serviceDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sd, 0755); err != nil {
		return fmt.Errorf("userservice: create services dir: %w", err)
	}
	return os.WriteFile(pp, []byte(strconv.Itoa(pid)), 0644)
}

// removePidFile removes the pidfile for name.
func removePidFile(name string) error {
	pp, err := pidPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(pp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("userservice: remove pidfile %s: %w", name, err)
	}
	return nil
}

// ---- internal helpers ----

// buildCmd builds an *exec.Cmd from a serviceConfig.
//
// It prefers the Args slice stored in the config (type-safe, handles
// arguments with spaces correctly).  For configs created before Args
// was added, it falls back to splitting ExecStart with strings.Fields
// (which works correctly as long as arguments do not contain spaces).
func buildCmd(cfg *serviceConfig) (*exec.Cmd, error) {
	if len(cfg.Args) > 0 {
		return exec.Command(cfg.Args[0], cfg.Args[1:]...), nil
	}
	// Backward compat: configs created before Args was added.
	parts := strings.Fields(cfg.ExecStart)
	if len(parts) == 0 {
		return nil, fmt.Errorf("userservice: empty ExecStart in config for %s", cfg.Name)
	}
	return exec.Command(parts[0], parts[1:]...), nil
}
