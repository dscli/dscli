//go:build darwin

package userservice

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ---- platform implementations ----

func create(name, desc string, cmd *exec.Cmd) error {
	hd, err := homeDir()
	if err != nil {
		return err
	}

	plistDir := filepath.Join(hd, "Library", "LaunchAgents")
	plistPath := filepath.Join(plistDir, name+".plist")

	content := formatLaunchdPlist(name, cmd.Args)

	// Idempotent: skip if file exists with identical content.
	if existing, err := os.ReadFile(plistPath); err == nil && string(existing) == content {
		return nil
	}

	if err := os.MkdirAll(plistDir, 0o700); err != nil {
		return fmt.Errorf("userservice: create LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("userservice: write plist: %w", err)
	}

	return nil
}

func start(name string) error {
	hd, err := homeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(hd, "Library", "LaunchAgents", name+".plist")

	// launchctl load loads the job and starts it (RunAtLoad=true).
	// If already loaded, it prints a warning but exits 0 — idempotent.
	return launchctl("load", plistPath)
}

func stop(name string) error {
	hd, err := homeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(hd, "Library", "LaunchAgents", name+".plist")

	// launchctl unload stops and unloads the job.
	return launchctl("unload", plistPath)
}

func deleteSv(name string) error {
	hd, err := homeDir()
	if err != nil {
		return err
	}

	plistPath := filepath.Join(hd, "Library", "LaunchAgents", name+".plist")

	// Unload first (best-effort, ignore errors).
	launchctl("unload", plistPath)

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("userservice: remove plist: %w", err)
	}
	return nil
}

func isRunning(name string) bool {
	return isLoaded(name)
}

// scan discovers LaunchAgent plists managed by dscli and returns the names
// of those that have no corresponding JSON registry entry (orphaned services).
func scan() ([]string, error) {
	hd, err := homeDir()
	if err != nil {
		return nil, err
	}

	plistDir := filepath.Join(hd, "Library", "LaunchAgents")
	entries, err := os.ReadDir(plistDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("userservice: read LaunchAgents dir: %w", err)
	}

	marker := []byte("Managed by dscli")

	var orphaned []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name, ok := strings.CutSuffix(e.Name(), ".plist")
		if !ok || name == "" {
			continue
		}

		plistPath := filepath.Join(plistDir, e.Name())
		data, err := os.ReadFile(plistPath)
		if err != nil {
			continue
		}

		if !bytes.Contains(data, marker) {
			continue
		}

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

// ---- launchctl helpers ----

// launchctl runs launchctl with the given arguments.
func launchctl(args ...string) error {
	cmd := exec.Command("launchctl", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// isLoaded reports whether the named LaunchAgent is loaded in launchd.
func isLoaded(label string) bool {
	return exec.Command("launchctl", "list", label).Run() == nil
}

// ---- plist formatting ----

// formatLaunchdPlist generates a LaunchAgent plist for the given service.
// args is the full argument list including argv[0] (the resolved binary path).
func formatLaunchdPlist(label string, args []string) string {
	var argsXML strings.Builder
	for _, a := range args {
		argsXML.WriteString(fmt.Sprintf("        <string>%s</string>\n", escapeXML(a)))
	}

	return fmt.Sprintf(`<!-- Managed by dscli -->
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
%s    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <false/>
</dict>
</plist>
`, escapeXML(label), argsXML.String())
}

// escapeXML escapes a string for inclusion in XML/Plist content.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
