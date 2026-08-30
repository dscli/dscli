//go:build windows

package keeprunning

import (
	"log/slog"

	"golang.org/x/sys/windows"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// keepRunning tells Windows not to put the system to sleep while this process
// runs. ES_CONTINUOUS resets the idle timer continuously; ES_SYSTEM_REQUIRED
// forces the display system to stay on.
//
// Ported from Pulumi's pkg/util/nosleep (Apache-2.0).
func keepRunning() DoneFunc {
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	setThreadExecutionState := kernel32.NewProc("SetThreadExecutionState")

	if r, _, _ := setThreadExecutionState.Call(
		uintptr(esContinuous | esSystemRequired),
	); r == 0 {
		slog.Info("keeprunning: SetThreadExecutionState failed, system may idle-sleep during long run")
		return func() {}
	}
	slog.Info("keeprunning: SetThreadExecutionState keeping system awake")

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		setThreadExecutionState.Call(uintptr(esContinuous))
		slog.Info("keeprunning: reset thread execution state")
	}
}
