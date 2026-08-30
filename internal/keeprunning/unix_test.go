//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly

package keeprunning

import (
	"errors"
	"os"
	"testing"

	"github.com/godbus/dbus/v5"
)

// TestKeepRunningNoBus verifies that a missing D-Bus session (headless CI or
// no desktop) yields a non-nil no-op DoneFunc that is safe to call.
func TestKeepRunningNoBus(t *testing.T) {
	old := sessionBus
	sessionBus = func() (*dbus.Conn, error) { return nil, errors.New("no bus") }
	t.Cleanup(func() { sessionBus = old })

	done := KeepRunning()
	if done == nil {
		t.Fatal("KeepRunning returned nil DoneFunc; want non-nil no-op")
	}
	done() // must not panic
}

// TestKeepRunningRealBus exercises the real inhibit/uninhibit path when a
// D-Bus session bus is available (e.g. a local KDE/GNOME desktop). It
// immediately releases the lock, so the test has no lasting side effect.
func TestKeepRunningRealBus(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus")
	}

	done := KeepRunning()
	if done == nil {
		t.Fatal("KeepRunning returned nil DoneFunc; want non-nil")
	}
	done()
}
