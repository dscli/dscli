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

func listSv() ([]string, error) {
	return nil, ErrUnsupported
}

func stale(name string) bool {
	return true
}

func status(name string) (string, error) {
	return "unsupported", nil
}
