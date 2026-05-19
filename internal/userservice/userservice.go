package userservice

import (
	"errors"
	"fmt"
	"os"
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

// homeDir returns the current user's home directory.
func homeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("userservice: cannot determine home directory: %w", err)
	}
	return dir, nil
}
