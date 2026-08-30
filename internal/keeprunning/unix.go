//go:build linux || freebsd || netbsd || openbsd || solaris || dragonfly

// Portions of this file are derived from Pulumi's pkg/util/nosleep
// (https://github.com/pulumi/pulumi/tree/master/pkg/util/nosleep), Copyright
// 2016-2024, Pulumi Corporation, licensed under the Apache License, Version
// 2.0 (the "License"); you may not use this file except in compliance with
// the License. You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package keeprunning

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	// dbusCallTimeout bounds every inhibit/uninhibit D-Bus call so a hung
	// desktop daemon cannot block chat/webchat startup or shutdown.
	dbusCallTimeout = 2 * time.Second

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
//
// ScreenSaver is tried first because KDE Plasma (Wayland) implements the
// xdg-screensaver interface but not org.gnome.SessionManager; upstream nosleep
// is GNOME-first - do not "fix" the order.
func keepRunning() DoneFunc {
	conn, err := sessionBus()
	if err != nil {
		slog.Info("keeprunning: no session bus, screen may idle-lock during long run", "err", err)
		return func() {}
	}

	// conn.Close ownership stays in keepRunning: the wrapper returned below
	// closes it exactly once after the release call.
	if done, ok := inhibitScreenSaver(conn); ok {
		return withConnClose(conn, done)
	}
	if done, ok := inhibitGnomeSession(conn); ok {
		return withConnClose(conn, done)
	}

	conn.Close()
	slog.Info("keeprunning: no supported inhibit interface, screen may idle-lock during long run")
	return func() {}
}

// withConnClose wraps a release function so the D-Bus connection is closed
// exactly once, after the release call, even if the returned DoneFunc is
// invoked concurrently or more than once.
func withConnClose(conn *dbus.Conn, done DoneFunc) DoneFunc {
	var once sync.Once
	return func() {
		once.Do(func() {
			done()
			conn.Close()
		})
	}
}

// inhibitScreenSaver uses org.freedesktop.ScreenSaver.Inhibit, the
// xdg-screensaver protocol supported by KDE, XFCE, Cinnamon and MATE.
func inhibitScreenSaver(conn *dbus.Conn) (DoneFunc, bool) {
	obj := conn.Object(screenSaverDest, screenSaverPath)
	var cookie uint32
	ctx, cancel := context.WithTimeout(context.Background(), dbusCallTimeout)
	defer cancel()
	err := obj.CallWithContext(ctx, screenSaverIface+".Inhibit", 0,
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
		ctx, cancel := context.WithTimeout(context.Background(), dbusCallTimeout)
		defer cancel()
		releaseErr := obj.CallWithContext(ctx, screenSaverIface+".UnInhibit", 0, cookie).Err
		if releaseErr != nil {
			slog.Info("keeprunning: failed to un-inhibit screen saver", "cookie", cookie, "err", releaseErr)
		} else {
			slog.Info("keeprunning: un-inhibited screen saver", "cookie", cookie)
		}
	}, true
}

// inhibitGnomeSession uses org.gnome.SessionManager.Inhibit. Flags 8 (idle)
// and 16 (no blanking) together block the lock-screen chain.
func inhibitGnomeSession(conn *dbus.Conn) (DoneFunc, bool) {
	obj := conn.Object(gnomeSessionDest, gnomeSessionPath)
	const (
		appName = "dscli"
		reason  = "dscli is running"
		// xid is a conventional non-zero sentinel used when there is no real
		// X window; some implementations reject 0.
		xid = uint32(42)
		// flags uses only the official GNOME gsm-inhibit-flags idle bit (8),
		// which already covers the idle -> blank -> lock chain. Bit 16 is not
		// part of the formal enum; some GNOME implementations reject unknown
		// bits and would fail the whole call, losing inhibition entirely. Add
		// a blanking bit here only if a future GNOME version proves to need it.
		flags = uint32(8)
	)
	var cookie uint32
	ctx, cancel := context.WithTimeout(context.Background(), dbusCallTimeout)
	defer cancel()
	err := obj.CallWithContext(ctx, gnomeSessionIface+".Inhibit", 0,
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
		ctx, cancel := context.WithTimeout(context.Background(), dbusCallTimeout)
		defer cancel()
		releaseErr := obj.CallWithContext(ctx, gnomeSessionIface+".Uninhibit", 0, cookie).Err
		if releaseErr != nil {
			slog.Info("keeprunning: failed to un-inhibit GNOME session", "cookie", cookie, "err", releaseErr)
		} else {
			slog.Info("keeprunning: un-inhibited GNOME session", "cookie", cookie)
		}
	}, true
}
