//go:build unix

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

func ResolveSocketPath(xdgRuntimeDir string, uid int) string {
	if xdgRuntimeDir != "" && filepath.IsAbs(xdgRuntimeDir) {
		return filepath.Join(xdgRuntimeDir, "a2ui", "a2ui.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("a2ui-%d", uid), "a2ui.sock")
}

func ListenUnix(path string) (*net.UnixListener, *Error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, NewError("ipc.invalid_socket_path", "Unix socket path must be absolute")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}

	// Serialize stale-socket probing, removal and bind for this pathname.
	// Without this lock, two concurrent daemon startups can both observe the
	// same stale socket, then one can unlink the live socket just created by
	// the other. The lock file intentionally remains on disk: unlinking it
	// would allow a third process to lock a different inode while a waiter
	// still holds the original one. Advisory locks are released automatically
	// when the process exits.
	lockPath := path + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}
	defer lockFile.Close()
	if err := os.Chmod(lockPath, 0o600); err != nil {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) // best-effort; Close also releases it

	if info, err := os.Lstat(path); err == nil {
		conn, dialErr := net.DialTimeout("unix", path, 100*time.Millisecond)
		if dialErr == nil {
			_ = conn.Close()
			return nil, NewError("ipc.daemon_already_running", "an A2UI daemon is already listening on the socket")
		}
		if info.Mode()&os.ModeSocket == 0 {
			return nil, NewError("ipc.socket_path_occupied", "socket path exists and is not a Unix socket")
		}
		if err := os.Remove(path); err != nil {
			return nil, NewError("ipc.socket_setup_failed", err.Error())
		}
	} else if !os.IsNotExist(err) {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}

	addr := &net.UnixAddr{Name: path, Net: "unix"}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}
	ln.SetUnlinkOnClose(true)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, NewError("ipc.socket_setup_failed", err.Error())
	}
	return ln, nil
}
