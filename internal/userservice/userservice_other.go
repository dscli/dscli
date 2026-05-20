//go:build !linux && !darwin

package userservice

import "os/exec"

func create(name, desc string, cmd *exec.Cmd) error {
	// No native service manager. Config is saved by the public Create.
	return nil
}

func start(name string) error {
	f := fallback{}
	return f.start(name)
}

func stop(name string) error {
	f := fallback{}
	return f.stop(name)
}

func deleteSv(name string) error {
	f := fallback{}
	return f.delete(name)
}

func isRunning(name string) bool {
	f := fallback{}
	return f.isRunning(name)
}

// scan returns no orphaned services on platforms without a native service manager.
func scan() ([]string, error) {
	return nil, nil
}
