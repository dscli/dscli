package userservice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUnsupported is returned when the platform has no service manager backend.
var ErrUnsupported = errors.New("userservice: platform not supported")

// Create creates or updates a user service configuration.
//
// On Linux: writes a systemd user unit file at
// ~/.config/systemd/user/<name>.service and runs daemon-reload + enable.
//
// On macOS: writes a LaunchAgent plist at
// ~/Library/LaunchAgents/<name>.plist.
//
// Create is idempotent: if the service file already exists with identical
// content, the file is not rewritten and no reload commands are run.
//
// Parameters:
//   - name: service name, used as filename stem and service identifier
//   - desc: human-readable description (systemd Description / Launchd Label)
//   - execStart: command line to execute, e.g. "/usr/bin/foo serve --host 127.0.0.1 --port 80"
func Create(name, desc, execStart string) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	if execStart == "" {
		return fmt.Errorf("userservice: execStart is required")
	}
	return create(name, desc, execStart)
}

// Start starts the user service.
//
// On Linux: runs "systemctl --user start <name>".
// On macOS: runs "launchctl load <plist-path>" (loads and starts the job).
func Start(name string) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	return start(name)
}

// Stop stops the user service.
//
// On Linux: runs "systemctl --user stop <name>".
// On macOS: runs "launchctl unload <plist-path>" (stops and unloads the job).
func Stop(name string) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	return stop(name)
}

// Delete removes the user service configuration and stops the service if
// it is running.
//
// On Linux: runs "systemctl --user disable --now <name>" and removes the
// unit file, then daemon-reload.
// On macOS: runs "launchctl unload" and removes the plist file.
func Delete(name string) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	return deleteSv(name)
}

// List returns the names of all services managed by userservice on this
// platform.
//
// On Linux: lists *.service files in ~/.config/systemd/user/.
// On macOS: lists *.plist files in ~/Library/LaunchAgents/.
func List() ([]string, error) {
	return listSv()
}

// Status returns a summary of the service's state:
//
//   - "running"   — service is active and config is fresh
//   - "stale"     — config is out of date (service may or may not be running)
//   - "stopped"   — config exists and is fresh, but service is not running
//   - "not_found" — no service config found for this name
//
// Status returns an error only when it cannot determine the state
// (e.g. home directory unavailable). On unsupported platforms it
// returns ("unsupported", nil).
func Status(name string) (string, error) {
	if name == "" {
		return "not_found", nil
	}
	return status(name)
}

// homeDir returns the current user's home directory.
func homeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("userservice: cannot determine home directory: %w", err)
	}
	return dir, nil
}

// staleCheck reports whether the config file at cfgPath is older than the
// dscli binary or the dscli config file (~/.dscli/config.dscli).
func staleCheck(cfgPath string) bool {
	cfgInfo, err := os.Stat(cfgPath)
	if err != nil {
		return true // can't stat → treat as stale
	}
	cfgModTime := cfgInfo.ModTime()

	// Check against dscli binary.
	if exePath, err := os.Executable(); err == nil {
		if exeInfo, err := os.Stat(exePath); err == nil {
			if exeInfo.ModTime().After(cfgModTime) {
				return true
			}
		}
	}

	// Check against dscli config file.
	if hd, err := os.UserHomeDir(); err == nil {
		dscliCfg := filepath.Join(hd, ".dscli", "config.dscli")
		if dscliInfo, err := os.Stat(dscliCfg); err == nil {
			if dscliInfo.ModTime().After(cfgModTime) {
				return true
			}
		}
	}

	return false
}
