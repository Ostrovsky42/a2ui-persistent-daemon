//go:build unix

package ipc

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func shortSocketTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "air-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestResolveSocketPathPrefersXDGAndFallsBackPerUID(t *testing.T) {
	if got := ResolveSocketPath("/run/user/1000", 1000); got != "/run/user/1000/a2ui/a2ui.sock" {
		t.Fatalf("unexpected XDG path %q", got)
	}
	got := ResolveSocketPath("", 4242)
	want := filepath.Join(os.TempDir(), "a2ui-4242", "a2ui.sock")
	if got != want {
		t.Fatalf("unexpected fallback path %q, want %q", got, want)
	}
}

func TestListenUnixSecuresDirectoryAndSocket(t *testing.T) {
	path := filepath.Join(shortSocketTempDir(t), "runtime", "a2ui.sock")
	ln, perr := ListenUnix(path)
	if perr != nil {
		t.Fatalf("listen: %v", perr)
	}
	defer ln.Close()

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("runtime dir mode = %o, want 700", got)
	}
	sockInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := sockInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %o, want 600", got)
	}
}

func TestListenUnixDoesNotRemoveLiveDaemonSocket(t *testing.T) {
	path := filepath.Join(shortSocketTempDir(t), "a2ui.sock")
	first, perr := ListenUnix(path)
	if perr != nil {
		t.Fatal(perr)
	}
	defer first.Close()

	second, perr := ListenUnix(path)
	if second != nil {
		second.Close()
		t.Fatal("second listener unexpectedly succeeded")
	}
	if perr == nil || perr.Code != "ipc.daemon_already_running" {
		t.Fatalf("expected daemon_already_running, got %+v", perr)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("live socket path was removed: %v", err)
	}
}

func TestListenUnixRecoversStaleSocket(t *testing.T) {
	path := filepath.Join(shortSocketTempDir(t), "a2ui.sock")
	addr := &net.UnixAddr{Name: path, Net: "unix"}
	stale, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	stale.SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected stale socket: %v", err)
	}

	ln, perr := ListenUnix(path)
	if perr != nil {
		t.Fatalf("recover stale: %v", perr)
	}
	defer ln.Close()
	if !strings.HasSuffix(ln.Addr().String(), "a2ui.sock") {
		t.Fatalf("unexpected listener addr %q", ln.Addr().String())
	}
}

func TestConcurrentListenUnixNeverUnlinksWinningLiveDaemon(t *testing.T) {
	path := filepath.Join(shortSocketTempDir(t), "a2ui.sock")
	const contenders = 12
	type result struct {
		ln   *net.UnixListener
		perr *Error
	}
	start := make(chan struct{})
	results := make(chan result, contenders)
	for i := 0; i < contenders; i++ {
		go func() {
			<-start
			ln, perr := ListenUnix(path)
			results <- result{ln: ln, perr: perr}
		}()
	}
	close(start)
	var winner *net.UnixListener
	for i := 0; i < contenders; i++ {
		r := <-results
		if r.ln != nil {
			if winner != nil {
				r.ln.Close()
				t.Fatal("more than one concurrent daemon acquired the same socket path")
			}
			winner = r.ln
		}
	}
	if winner == nil {
		t.Fatal("no daemon acquired socket")
	}
	defer winner.Close()
	conn, err := net.DialTimeout("unix", path, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("winning live daemon socket was unlinked or became unreachable: %v", err)
	}
	conn.Close()
}
