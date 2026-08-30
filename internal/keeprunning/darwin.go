//go:build darwin

package keeprunning

import (
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// keepRunning spawns `caffeinate -i -w <pid>` to keep the system awake while
// this process runs. `-i` prevents idle sleep, `-w <pid>` makes caffeinate
// exit automatically when the given process terminates.
//
// Ported from Pulumi's pkg/util/nosleep (Apache-2.0).
func keepRunning() DoneFunc {
	cmd := exec.Command("caffeinate", "-i", "-w", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		slog.Info("keeprunning: caffeinate unavailable, screen may idle-lock during long run", "err", err)
		return func() {}
	}
	slog.Info("keeprunning: caffeinate keeping system awake")

	var released bool
	return func() {
		if released {
			return
		}
		released = true
		if err := cmd.Process.Kill(); err != nil {
			slog.Info("keeprunning: failed to stop caffeinate", "err", err)
		} else {
			slog.Info("keeprunning: stopped caffeinate")
		}
		_ = cmd.Wait()
	}
}
