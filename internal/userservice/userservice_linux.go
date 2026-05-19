//go:build linux

package userservice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---- platform implementations ----

func create(name, desc, execStart string) error {
	if !systemdUserAvailable() {
		return ErrUnsupported
	}

	hd, err := homeDir()
	if err != nil {
		return err
	}

	unitDir := filepath.Join(hd, ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, name+".service")

	content := formatSystemdUnit(desc, execStart)

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
		return ErrUnsupported
	}

	// If already active, no-op.
	if systemctlIsActive(name) {
		return nil
	}

	return systemctl("start", name)
}

func stop(name string) error {
	if !systemdUserAvailable() {
		return ErrUnsupported
	}

	// systemctl stop on an inactive service exits 0, so no pre-check needed.
	return systemctl("stop", name)
}

func deleteSv(name string) error {
	if !systemdUserAvailable() {
		return ErrUnsupported
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

func listSv() ([]string, error) {
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
		return nil, fmt.Errorf("userservice: read unit dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if name, ok := strings.CutSuffix(e.Name(), ".service"); ok && name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func stale(name string) bool {
	hd, err := homeDir()
	if err != nil {
		return true
	}
	unitPath := filepath.Join(hd, ".config", "systemd", "user", name+".service")
	return staleCheck(unitPath)
}

func status(name string) (string, error) {
	if stale(name) {
		return "stale", nil
	}
	if systemdUserAvailable() && systemctlIsActive(name) {
		return "running", nil
	}
	return "stopped", nil
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
	return fmt.Sprintf(`[Unit]
Description=%s

[Service]
Type=simple
ExecStart=%s
Restart=no

[Install]
WantedBy=default.target
`, desc, execStart)
}
