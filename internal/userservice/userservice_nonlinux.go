//go:build !linux

package userservice

// systemdUserAvailable reports whether the systemd user instance is reachable.
// Non-Linux platforms have no systemd, so this is always false. It exists to
// satisfy tests that reference it on every platform (see fallback_test.go).
func systemdUserAvailable() bool { return false }
