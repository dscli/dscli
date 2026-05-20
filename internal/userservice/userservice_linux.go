//go:build linux

package userservice

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---- platform implementations ----

func create(name, desc string, cmd *exec.Cmd) error {
	if !systemdUserAvailable() {
		// Fallback: no systemd available. Config is saved by the public
		// Create function; daemonization happens at Start time.
		return nil
	}

	hd, err := homeDir()
	if err != nil {
		return err
	}

	unitDir := filepath.Join(hd, ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, name+".service")

	content := formatSystemdUnit(desc, cmd.String())

	// Idempotent: skip if file exists with identical content.
	if existing, err := os.ReadFile(unitPath); err == nil && string(existing) == content {
		return nil
	}

	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("userservice: create unit dir: %w", err)
	}
	if err := os.WriteFile(unitPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("userservice: write unit file: %w", err)
	}

	// daemon-reload so systemd picks up the new/changed unit.
	if err := systemctl("daemon-reload"); err != nil {
		return fmt.Errorf("userservice: daemon-reload: %w", err)
	}
	// enable so the service starts at login.
	if err := systemctl("enable", name); err != nil {
		return fmt.Errorf("userservice: enable: %w", err)
	}

	return nil
}

func start(name string) error {
	if !systemdUserAvailable() {
		f := fallback{}
		return f.start(name)
	}

	// If already active, no-op.
	if systemctlIsActive(name) {
		return nil
	}

	return systemctl("start", name)
}

func stop(name string) error {
	if !systemdUserAvailable() {
		f := fallback{}
		return f.stop(name)
	}

	// systemctl stop on an inactive service exits 0, so no pre-check needed.
	return systemctl("stop", name)
}

func deleteSv(name string) error {
	if !systemdUserAvailable() {
		f := fallback{}
		return f.delete(name)
	}

	hd, err := homeDir()
	if err != nil {
		return err
	}

	unitPath := filepath.Join(hd, ".config", "systemd", "user", name+".service")

	// Stop and disable the service if it exists.
	systemctl("disable", "--now", name) // best-effort, ignore errors

	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("userservice: remove unit file: %w", err)
	}

	return systemctl("daemon-reload")
}

func isRunning(name string) bool {
	if !systemdUserAvailable() {
		f := fallback{}
		return f.isRunning(name)
	}
	return systemctlIsActive(name)
}

// scan discovers systemd user units managed by dscli and returns the names
// of those that have no corresponding JSON registry entry (orphaned services).
func scan() ([]string, error) {
	if !systemdUserAvailable() {
		return nil, nil
	}

	hd, err := homeDir()
	if err != nil {
		return nil, err
	}

	unitDir := filepath.Join(hd, ".config", "systemd", "user")
	entries, err := os.ReadDir(unitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("userservice: read systemd user dir: %w", err)
	}

	marker := []byte("Managed by dscli")

	var orphaned []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".service")
		if !ok || name == "" {
			continue
		}

		unitPath := filepath.Join(unitDir, e.Name())
		data, err := os.ReadFile(unitPath)
		if err != nil {
			continue // skip unreadable files
		}

		if !bytes.Contains(data, marker) {
			continue
		}

		// Found a dscli-managed unit. Check if JSON registry exists.
		cfgPath, err := serviceConfigPath(name)
		if err != nil {
			orphaned = append(orphaned, name)
			continue
		}
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			orphaned = append(orphaned, name)
		}
	}

	return orphaned, nil
}

// ---- systemd helpers ----

// systemdUserAvailable reports whether the systemd user instance is reachable.
func systemdUserAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "--no-pager")
	return cmd.Run() == nil
}

// systemctlIsActive reports whether the named systemd user service is active.
func systemctlIsActive(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "is-active", name)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

// systemctl runs systemctl --user with the given arguments.
// Output is redirected to stderr.
func systemctl(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// formatSystemdUnit generates the content of a systemd user unit file.
func formatSystemdUnit(desc, execStart string) string {
	return fmt.Sprintf(`# Managed by dscli
[Unit]
Description=%s

[Service]
Type=simple
ExecStart=%s
Restart=no

[Install]
WantedBy=default.target
`, desc, execStart)
}
