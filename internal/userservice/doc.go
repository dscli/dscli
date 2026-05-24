// Package userservice manages OS-level user services (daemons that run
// as the current user, independent of the calling process lifecycle).
//
// # Architecture
//
// The userservice package provides a unified API across all platforms:
//
//	Create(name, desc, cmd) error
//	Start(name) error
//	Stop(name) error
//	Status(name) (string, error)
//	List() ([]string, error)
//	Delete(name) error
//	Scan() ([]string, error)
//	ScanStatus(name) (string, error)
//
// Platform backends:
//
//	Linux      systemd --user   → pidfile fallback if systemd unavailable
//	macOS      launchctl        → no fallback needed
//	Other      none             → pidfile + direct process daemon
//
// Registry: ~/.dscli/services/<name>.json
// Marker:   "# Managed by dscli" / "<!-- Managed by dscli -->"
//
// # Backends
//
//	Platform   Primary Backend    Fallback              Config Directory
//	Linux      systemd --user     pidfile (if no        ~/.config/systemd/user/
//	                              systemd available)    ~/.dscli/services/
//	macOS      launchctl          n/a                   ~/Library/LaunchAgents/
//	                                                   ~/.dscli/services/
//	Other      n/a                pidfile + direct      ~/.dscli/services/
//	                              process daemon
//
// # Fallback
//
// When systemd is unavailable on Linux, or on any non-Linux, non-macOS
// platform (Windows, FreeBSD, etc.), userservice falls back to direct
// process management:
//
//   - Start: daemonizes the command (Setsid / CREATE_NEW_PROCESS_GROUP),
//     redirects stdio to /dev/null, records PID in
//     ~/.dscli/services/<name>.pid.
//   - Stop: reads PID from pidfile, sends SIGTERM (taskkill on Windows),
//     removes pidfile.
//   - Status: checks process liveness via signal(0) (tasklist on Windows).
//     Stale pidfiles are auto-cleaned.
//
// All fallback operations are idempotent: calling start on a running
// service is a no-op, calling stop on a stopped service succeeds.
//
// # Registry
//
// Every service created through userservice is recorded in a JSON
// registry at ~/.dscli/services/<name>.json:
//
//	{
//	  "name": "my-service",
//	  "desc": "My Service",
//	  "exec_start": "/usr/bin/my-service --flag",
//	  "args": ["/usr/bin/my-service", "--flag"]
//	}
//
// The registry is the source of truth for List, Status, and Delete.
// The Args field (stored verbatim from *exec.Cmd.Args) enables type-safe
// command reconstruction without fragile whitespace splitting.
//
// # DScli Marker
//
// Service unit files include a dscli marker for identification:
//
//	systemd:  # Managed by dscli     (first line of unit file)
//	launchd:  <!-- Managed by dscli --> (first line of plist)
//
// The Scan function uses this marker to discover orphaned services
// (OS-level units without a corresponding JSON registry entry).
// Services created before the marker was introduced can be re-registered
// by running Create again (it is idempotent).
//
// # API
//
// Create writes the platform-specific service configuration (systemd unit
// file, LaunchAgent plist, or registry-only for fallback platforms) and
// records it in the JSON registry.  It resolves the binary path via
// LookPath so the config always carries an absolute path.
//
// Create is idempotent: if the service file already exists with identical
// content, no changes are made.  The JSON registry is always refreshed.
//
// Start activates the service.  On systemd: runs "systemctl --user start
// <name>".  On macOS: runs "launchctl load <plist>" (RunAtLoad ensures
// the job starts).  On fallback: daemonizes the command and records the
// PID.
//
// Stop deactivates the service.  On systemd: runs "systemctl --user stop
// <name>".  On macOS: runs "launchctl unload <plist>".  On fallback:
// sends SIGTERM/taskkill and removes the pidfile.
//
// Delete removes all traces of the service: stops it if running, removes
// the platform-specific config files, and deletes the JSON registry entry
// (best-effort).
//
// List returns the names of all dscli-managed services by scanning
// ~/.dscli/services/*.json.  Returns an empty slice (not nil) when no
// services exist.
//
// Status reports one of:
//
//	"running"   — service is active and config is fresh
//	"stale"     — config is out of date (dscli binary or config newer)
//	"stopped"   — config exists and is fresh, but service is not running
//	"not_found" — no registry entry for this name
//
// "stale" indicates the service was created by an older dscli version or
// before a config change; it should be re-created to pick up updates.
//
// Scan returns the names of services that exist at the OS level (systemd/
// launchd units with the dscli marker) but lack a JSON registry entry.
// These "orphaned" services were likely created before the JSON registry
// was introduced, or their registry files were deleted.
//
// ScanStatus is like Status but works even when the JSON registry entry
// is missing.  It checks the OS-level service manager directly.  Unlike
// Status, ScanStatus never returns "stale" (stale detection requires a
// registry entry).
//
// # Design Decisions
//
//  1. Why not use github.com/kardianos/service?
//     That library focuses on system services (root-level daemons) and carries
//     significant complexity.  userservice focuses exclusively on user-scoped
//     services with a minimal API surface.
//
//  2. Create takes *exec.Cmd, not a string.
//     This avoids fragile string parsing of command lines (no shell-quoting or
//     whitespace-splitting ambiguity).  The public Create resolves cmd.Path via
//     LookPath and persists cmd.Args verbatim so every backend reconstructs the
//     command correctly.
//
//  3. Create does NOT start the service.
//     Create and Start are separate calls so callers can decide whether to start
//     immediately or just ensure the config is deployed.
//
//  4. fallback is not a global singleton.
//     Each call constructs a new fallback{} instance, which carries no mutable
//     state (all state lives in the pidfile and JSON registry on disk).
//
// # Usage
//
//	import "gitcode.com/dscli/dscli/internal/userservice"
//
//	cmd := exec.Command("/usr/bin/lightpanda", "serve", "--host", "127.2.2.9", "--port", "9227")
//	if err := userservice.Create("dscli-lightpanda", "Lightpanda Browser", cmd); err != nil {
//	    // handle
//	}
//	if err := userservice.Start("dscli-lightpanda"); err != nil {
//	    // handle
//	}
package userservice