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
//   - desc: human-readable description (systemd Description / launchd Label)
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

// Refresh recreates a service from its registry entry (~/.dscli/services/<name>.json).
//
// This is useful when the dscli binary or config file has been updated
// after the service was created, making the OS-level unit file stale.
// Refresh reads the stored name/desc/args and calls Create with the same
// parameters, which regenerates the systemd/launchd configuration.
//
// Refresh is idempotent: Create skips writing if the file content has not
// changed.
func Refresh(name string) error {
	cfg, err := loadServiceConfig(name)
	if err != nil {
		return err
	}
	cmd, err := buildCmd(cfg)
	if err != nil {
		return fmt.Errorf("userservice: refresh %s: %w", name, err)
	}
	return Create(name, cfg.Desc, cmd)
}

// Start auto-refreshes the service configuration if it is stale
// (dscli or config updated since the service was last created), then
// starts the service.
func Start(name string) error {
	if name == "" {
		return fmt.Errorf("userservice: name is required")
	}
	// Auto-refresh if stale — the registry has all the info we need.
	if s, _ := Status(name); s == "stale" {
		_ = Refresh(name)
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
//   - "running"   — service is active
//   - "stopped"   — service config exists but is not running
//   - "not_found" — no service config found for this name
//   - "stale"     — (rare fallback) config is out of date and
//     auto-refresh failed; service may or may not be running
//
// "stale" is rarely returned because Status automatically refreshes
// stale configurations by recreating them from the registry entry.
// It is only returned when the auto-refresh itself fails AND the
// service is not running.
//
// Note on auto-refresh: refreshing the unit file does NOT restart the
// running service process.  A running service continues with the old
// binary/config until it is explicitly restarted.  For dscli-managed
// services this is acceptable because command-line arguments rarely
// change between versions.
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
		// Auto-refresh: recreate from registry (idempotent).
		if err := Refresh(name); err != nil {
			// Refresh failed. If the service is still running, report
			// it as running — the old config still works.
			if isRunning(name) {
				return "running", nil
			}
			return "stale", nil
		}
		// Successfully refreshed — re-check running state.
	}

	if isRunning(name) {
		return "running", nil
	}
	return "stopped", nil
}

// Scan returns the names of dscli-managed services that exist at the OS
// level (systemd/launchd) but have no corresponding JSON registry entry.
// These "orphaned" services were likely created before the JSON registry
// was introduced, or their registry files were deleted.
//
// Use the --scan flag on "dscli service list" or "dscli service status"
// to include orphaned services.  Orphaned services can be re-registered
// by running "dscli service create" again (it is idempotent).
func Scan() ([]string, error) {
	return scan()
}

// ScanStatus is like Status but works even when the JSON registry
// entry is missing.  It checks the OS-level service manager directly.
//
// Returns:
//   - "running"       — service is active at the OS level
//   - "stopped"       — service unit exists but is not running
//   - "not_found"     — no service found at the OS level either
//   - "stale"         — (never returned by ScanStatus — stale requires a registry entry)
func ScanStatus(name string) (string, error) {
	if name == "" {
		return "not_found", nil
	}

	// First try the standard Status (handles registered services).
	s, err := Status(name)
	if err != nil {
		return "", err
	}
	if s != "not_found" {
		return s, nil
	}

	// No JSON registry — check OS directly.
	if isRunning(name) {
		return "running", nil
	}

	// Check if the OS-level unit exists (even if not running).
	if unitExists(name) {
		return "stopped", nil
	}

	return "not_found", nil
}

// unitExists reports whether the OS-level service unit exists for name.
func unitExists(name string) bool {
	hd, err := homeDir()
	if err != nil {
		return false
	}

	// Check systemd unit file.
	unitPath := filepath.Join(hd, ".config", "systemd", "user", name+".service")
	if _, err := os.Stat(unitPath); err == nil {
		return true
	}

	// Check LaunchAgent plist.
	plistPath := filepath.Join(hd, "Library", "LaunchAgents", name+".plist")
	if _, err := os.Stat(plistPath); err == nil {
		return true
	}

	return false
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
	if err := os.MkdirAll(sd, 0o755); err != nil {
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
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
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
