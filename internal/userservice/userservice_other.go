//go:build !linux && !darwin

package userservice

import "fmt"

func create(name, desc, execStart string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, "only Linux (systemd) and macOS (launchctl) are supported")
}

func start(name string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, "only Linux (systemd) and macOS (launchctl) are supported")
}

func stop(name string) error {
	return fmt.Errorf("%w: %s", ErrUnsupported, "only Linux (systemd) and macOS (launchctl) are supported")
}
