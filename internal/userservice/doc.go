// Package userservice manages OS-level user services (daemons that run
// as the current user, independent of the calling process lifecycle).
//
// # Architecture
//
//	┌──────────────────────────────────────────┐
//	│            userservice package            │
//	│                                           │
//	│  Create(name, desc, execStart) error      │
//	│  Start(name) error                        │
//	│  Stop(name) error                         │
//	│                                           │
//	│  ┌──────────────┐  ┌────────────────────┐ │
//	│  │ linux        │  │ darwin             │ │
//	│  │ (systemd)    │  │ (launchctl)        │ │
//	│  │              │  │                    │ │
//	│  │ ~/.config/   │  │ ~/Library/         │ │
//	│  │ systemd/user │  │ LaunchAgents/      │ │
//	│  │ <name>.service│  │ <name>.plist       │ │
//	│  └──────────────┘  └────────────────────┘ │
//	└──────────────────────────────────────────┘
//
// # API
//
// Create writes the service configuration file and performs platform-specific
// registration (systemd enable, etc). It is idempotent: if the service file
// already exists with identical content, no action is taken.
//
// Start activates the service, launching the daemon process. On systemd this
// runs "systemctl --user start <name>". On macOS this runs "launchctl load
// <plist>", which both loads and starts the job (RunAtLoad is set to true).
//
// Stop deactivates the service. On systemd this runs "systemctl --user stop
// <name>". On macOS this runs "launchctl unload <plist>", which both stops
// and unloads the job. The plist remains in ~/Library/LaunchAgents/ and will
// be reloaded at next login.
//
// # Platform Support
//
//	Platform   Backend          Service Dir
//	Linux      systemd --user   ~/.config/systemd/user/
//	macOS      launchctl        ~/Library/LaunchAgents/
//	Other      unsupported      returns ErrUnsupported
//
// # Design Decisions
//
//  1. Why not use github.com/kardianos/service?
//     That library focuses on system services (root-level daemons) and carries
//     significant complexity. userservice focuses exclusively on user-scoped
//     services with a minimal API surface (3 functions).
//
//  2. execStart is a single string (systemd ExecStart format). On macOS it is
//     split by whitespace into ProgramArguments for the plist.
//
//  3. Create does NOT start the service. Create and Start are separate calls
//     so callers can decide whether to start immediately or just ensure the
//     config is deployed.
//
// # Usage
//
//	import "gitcode.com/dscli/dscli/internal/userservice"
//
//	execStart := "/usr/bin/lightpanda serve --host 127.2.2.9 --port 9227"
//	if err := userservice.Create("dscli-lightpanda", "Lightpanda Browser", execStart); err != nil {
//	    // handle
//	}
//	if err := userservice.Start("dscli-lightpanda"); err != nil {
//	    // handle
//	}
package userservice
