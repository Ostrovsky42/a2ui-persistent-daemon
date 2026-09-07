//go:build unix

package localenv

import "errors"

var ErrSupervisorAlreadyRunning = errors.New("supervisor already running")

type Lock struct{}

func AcquireUpLock(string) (*Lock, error) {
	return &Lock{}, nil
}

func TryAcquireSupervisorLock(string) (*Lock, error) {
	return &Lock{}, nil
}

func (l *Lock) Release() error {
	return nil
}
