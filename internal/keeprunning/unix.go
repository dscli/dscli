//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly

package keeprunning

import (
	"log/slog"

	"github.com/godbus/dbus/v5"
)

const (
	screenSaverDest   = "org.freedesktop.ScreenSaver"
	screenSaverPath   = "/ScreenSaver"
	screenSaverIface  = "org.freedesktop.ScreenSaver"
	gnomeSessionDest  = "org.gnome.SessionManager"
	gnomeSessionPath  = "/org/gnome/SessionManager"
	gnomeSessionIface = "org.gnome.SessionManager"
)

// sessionBus is injectable for tests.
var sessionBus = dbus.SessionBus

// keepRunning tries, in order, the desktop-independent ScreenSaver inhibit
// protocol (KDE, XFCE, Cinnamon, MATE) and then the GNOME SessionManager
// inhibit. The first successful mechanism wins; if none succeeds, it returns
// a no-op DoneFunc.
func keepRunning() DoneFunc {
	conn, err := sessionBus()
	if err != nil {
		slog.Info("keeprunning: no session bus, screen may idle-lock during long run", "err", err)
		return func() {}
	}

	if done, ok := inhibitScreenSaver(conn); ok {
		return done
	}
	if done, ok := inhibitGnomeSession(conn); ok {
		return done
	}

	conn.Close()
	slog.Info("keeprunning: no supported inhibit interface, screen may idle-lock during long run")
	return func() {}
}

// inhibitScreenSaver uses org.freedesktop.ScreenSaver.Inhibit, the
// xdg-screensaver protocol supported by KDE, XFCE, Cinnamon and MATE.
func inhibitScreenSaver(conn *dbus.Conn) (DoneFunc, bool) {
	obj := conn.Object(screenSaverDest, screenSaverPath)
	var cookie uint32
	err := obj.Call(screenSaverIface+".Inhibit", 0,
		"dscli", "dscli is running").Store(&cookie)
	if err != nil {
		return nil, false
	}
	slog.Info("keeprunning: inhibited screen saver", "mechanism", screenSaverIface, "cookie", cookie)

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		releaseErr := obj.Call(screenSaverIface+".UnInhibit", 0, cookie).Err
		if releaseErr != nil {
			slog.Info("keeprunning: failed to un-inhibit screen saver", "cookie", cookie, "err", releaseErr)
		} else {
			slog.Info("keeprunning: un-inhibited screen saver", "cookie", cookie)
		}
		conn.Close()
	}, true
}

// inhibitGnomeSession uses org.gnome.SessionManager.Inhibit. Flags 8 (idle)
// and 16 (no blanking) together block the lock-screen chain.
func inhibitGnomeSession(conn *dbus.Conn) (DoneFunc, bool) {
	obj := conn.Object(gnomeSessionDest, gnomeSessionPath)
	const (
		appName = "dscli"
		reason  = "dscli is running"
		xid     = uint32(42)
		flags   = uint32(8 | 16)
	)
	var cookie uint32
	err := obj.Call(gnomeSessionIface+".Inhibit", 0,
		appName, xid, reason, flags).Store(&cookie)
	if err != nil {
		return nil, false
	}
	slog.Info("keeprunning: inhibited GNOME session", "mechanism", gnomeSessionIface, "cookie", cookie)

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		releaseErr := obj.Call(gnomeSessionIface+".Uninhibit", 0, cookie).Err
		if releaseErr != nil {
			slog.Info("keeprunning: failed to un-inhibit GNOME session", "cookie", cookie, "err", releaseErr)
		} else {
			slog.Info("keeprunning: un-inhibited GNOME session", "cookie", cookie)
		}
		conn.Close()
	}, true
}
