//go:build unix

package localenv

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

var ErrSupervisorAlreadyRunning = errors.New("supervisor already running")

type Lock struct {
	mu       sync.Mutex
	file     *os.File
	released bool
}

func openLockFile(path string) (*os.File, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("lock path must be absolute: %q", path)
	}
	if err := ensurePrivateDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %q: %w", path, err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure lock %q: %w", path, err)
	}
	return file, nil
}

func AcquireUpLock(path string) (*Lock, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire up lock: %w", err)
	}
	return &Lock{file: file}, nil
}

func TryAcquireSupervisorLock(path string) (*Lock, error) {
	file, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrSupervisorAlreadyRunning
		}
		return nil, fmt.Errorf("acquire supervisor lock: %w", err)
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Release() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.released {
		return nil
	}
	l.released = true
	if l.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil
	if unlockErr != nil {
		return fmt.Errorf("unlock lifecycle lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close lifecycle lock: %w", closeErr)
	}
	return nil
}
