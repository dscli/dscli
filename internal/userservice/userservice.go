package userservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrUnsupported is returned when the platform has no service manager backend.
var ErrUnsupported = errors.New("userservice: platform not supported")

// serviceConfig is persisted to ~/.dscli/services/<name>.json as the
// source-of-truth registry of dscli-managed user services.
// serviceConfig is persisted to ~/.dscli/services/<name>.json as the
// source-of-truth registry of dscli-managed user services.
type serviceConfig struct {
	Name      string   `json:"name"`
	Desc      string   `json:"desc"`
	ExecStart string   `json:"exec_start"`
	Args      []string `json:"args,omitempty"`
}
// Create creates or updates a user service configuration.
//
// On Linux: writes a systemd user unit file at
// ~/.config/systemd/user/<name>.service, runs daemon-reload + enable,
// then records the config at ~/.dscli/services/<name>.json.
//
// On macOS: writes a LaunchAgent plist at
// ~/Library/LaunchAgents/<name>.plist, then records the config at
// ~/.dscli/services/<name>.json.
//
// Create is idempotent: if the service file already exists with identical
// content, the file is not rewritten and no reload commands are run.
// The JSON registry is always refreshed.
//
// Create resolves cmd.Path via LookPath so the service file always
// contains the absolute binary path.  cmd.Args[0] is rewritten to the
// resolved path; on Linux the command line uses cmd.String(), while on
// macOS cmd.Args is used as ProgramArguments directly (no fragile
// whitespace splitting).
//
// Parameters:
//   - name: service name, used as filename stem and service identifier
//   - desc: human-readable description (systemd Description / Launchd Label)
//   - cmd: command to execute; Path must be non-empty and resolvable
func Create(name, desc string, cmd *exec.Cmd) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	if cmd == nil {
		return fmt.Errorf("userservice: cmd is required")
	}
	if cmd.Path == "" {
		return fmt.Errorf("userservice: cmd.Path is required")
	}

	// Resolve binary path so the service file always carries the absolute
	// path regardless of the service manager's PATH configuration.
	resolved, err := exec.LookPath(cmd.Path)
	if err != nil {
		return fmt.Errorf("userservice: %s not found in PATH: %w", cmd.Path, err)
	}

	// Build a clean *exec.Cmd with the resolved path so cmd.String()
	// (Linux) and cmd.Args (macOS) both use the absolute binary location.
	resolvedCmd := &exec.Cmd{Path: resolved}
	if len(cmd.Args) > 0 {
		resolvedCmd.Args = make([]string, len(cmd.Args))
		copy(resolvedCmd.Args, cmd.Args)
		resolvedCmd.Args[0] = resolved
	} else {
		resolvedCmd.Args = []string{resolved}
	}

	execStart := resolvedCmd.String()

	if err := create(name, desc, resolvedCmd); err != nil {
		return err
	}
	return saveServiceConfig(name, desc, execStart, resolvedCmd.Args)
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
//
// The JSON registry entry at ~/.dscli/services/<name>.json is also
// removed (best-effort).
func Delete(name string) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	err := deleteSv(name)
	// Best-effort: remove registry even if deleteSv failed partially.
	_ = removeServiceConfig(name)
	return err
}

// List returns the names of all services managed by userservice.
//
// It reads the dscli registry at ~/.dscli/services/ — only services
// created through userservice.Create are listed.  Returns an empty
// slice (not nil) when the directory does not exist.
func List() ([]string, error) {
	sd, err := serviceDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(sd)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("userservice: read services dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if name, ok := strings.CutSuffix(e.Name(), ".json"); ok && name != "" {
			names = append(names, name)
		}
	}
	if names == nil {
		return []string{}, nil
	}
	return names, nil
}

// Status returns a summary of the service's state:
//
//   - "running"   — service is active and config is fresh
//   - "stale"     — config is out of date (service may or may not be running)
//   - "stopped"   — config exists and is fresh, but service is not running
//   - "not_found" — no service config found for this name
//
// "not_found" is returned when the registry entry
// (~/.dscli/services/<name>.json) is missing.  "stale" means the
// registry entry is older than the dscli binary or the dscli config
// file.
//
// Status returns an error only when it cannot determine the state
// (e.g. home directory unavailable).
func Status(name string) (string, error) {
	if name == "" {
		return "not_found", nil
	}

	cfgPath, err := serviceConfigPath(name)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return "not_found", nil
	}
	if staleCheck(cfgPath) {
		return "stale", nil
	}
	if isRunning(name) {
		return "running", nil
	}
	return "stopped", nil
}

// ---- internal helpers ----

// homeDir returns the current user's home directory.
func homeDir() (string, error) {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("userservice: cannot determine home directory: %w", err)
	}
	return dir, nil
}

// serviceDir returns the path to the dscli services registry directory.
func serviceDir() (string, error) {
	hd, err := homeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(hd, ".dscli", "services"), nil
}

// serviceConfigPath returns the path to the JSON registry entry for name.
func serviceConfigPath(name string) (string, error) {
	sd, err := serviceDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(sd, name+".json"), nil
}

// saveServiceConfig writes the registry entry to disk.
func saveServiceConfig(name, desc, execStart string, args []string) error {
	sd, err := serviceDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(sd, 0755); err != nil {
		return fmt.Errorf("userservice: create services dir: %w", err)
	}

	cfg := serviceConfig{
		Name:      name,
		Desc:      desc,
		ExecStart: execStart,
		Args:      args,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("userservice: marshal config: %w", err)
	}

	cfgPath := filepath.Join(sd, name+".json")
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		return fmt.Errorf("userservice: write config: %w", err)
	}
	return nil
}

// loadServiceConfig reads the registry entry for name from disk.
func loadServiceConfig(name string) (*serviceConfig, error) {
	cfgPath, err := serviceConfigPath(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("userservice: read config %s: %w", name, err)
	}
	var cfg serviceConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("userservice: parse config %s: %w", name, err)
	}
	return &cfg, nil
}

// removeServiceConfig removes the registry entry for name.
func removeServiceConfig(name string) error {
	cfgPath, err := serviceConfigPath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(cfgPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("userservice: remove config: %w", err)
	}
	return nil
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
