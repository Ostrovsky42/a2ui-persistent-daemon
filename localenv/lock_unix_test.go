//go:build unix

package localenv

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUpLockSerializesCallersAndUsesPrivateFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "up.lock")
	first, err := AcquireUpLock(path)
	if err != nil {
		t.Fatalf("AcquireUpLock(first): %v", err)
	}
	defer first.Release()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("up.lock permissions=%#o, want 0600", perm)
	}

	started := make(chan struct{})
	acquired := make(chan *Lock, 1)
	errCh := make(chan error, 1)
	go func() {
		close(started)
		lock, err := AcquireUpLock(path)
		if err != nil {
			errCh <- err
			return
		}
		acquired <- lock
	}()
	<-started

	select {
	case lock := <-acquired:
		_ = lock.Release()
		t.Fatal("second up.lock owner acquired before first released")
	case err := <-errCh:
		t.Fatalf("second AcquireUpLock returned early: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := first.Release(); err != nil {
		t.Fatalf("Release(first): %v", err)
	}
	select {
	case lock := <-acquired:
		if err := lock.Release(); err != nil {
			t.Fatalf("Release(second): %v", err)
		}
	case err := <-errCh:
		t.Fatalf("second AcquireUpLock: %v", err)
	case <-time.After(time.Second):
		t.Fatal("second up.lock owner did not acquire after release")
	}
}

func TestSupervisorLockRejectsSecondLifetimeOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "supervisor.lock")
	first, err := TryAcquireSupervisorLock(path)
	if err != nil {
		t.Fatalf("TryAcquireSupervisorLock(first): %v", err)
	}
	defer first.Release()

	second, err := TryAcquireSupervisorLock(path)
	if second != nil {
		_ = second.Release()
		t.Fatal("second supervisor lock unexpectedly acquired")
	}
	if !errors.Is(err, ErrSupervisorAlreadyRunning) {
		t.Fatalf("second supervisor lock error=%v, want ErrSupervisorAlreadyRunning", err)
	}

	if err := first.Release(); err != nil {
		t.Fatalf("Release(first): %v", err)
	}
	third, err := TryAcquireSupervisorLock(path)
	if err != nil {
		t.Fatalf("TryAcquireSupervisorLock(after release): %v", err)
	}
	if err := third.Release(); err != nil {
		t.Fatalf("Release(third): %v", err)
	}
}
