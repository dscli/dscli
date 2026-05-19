//go:build !linux && !darwin

package userservice

func create(name, desc, execStart string) error {
	return ErrUnsupported
}

func start(name string) error {
	return ErrUnsupported
}

func stop(name string) error {
	return ErrUnsupported
}

func deleteSv(name string) error {
	return ErrUnsupported
}

func isRunning(name string) bool {
	return false
}
