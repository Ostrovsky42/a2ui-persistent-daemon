//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"

	"a2ui/ipc"
)

func TestResolveDaemonSocketUsesExplicitThenXDG(t *testing.T) {
	if got := resolveDaemonSocket("/custom/a2ui.sock", "/run/user/1000", 1000); got != "/custom/a2ui.sock" {
		t.Fatalf("explicit socket=%q", got)
	}
	if got := resolveDaemonSocket("", "/run/user/1000", 1000); got != "/run/user/1000/a2ui/a2ui.sock" {
		t.Fatalf("XDG socket=%q", got)
	}
}

func TestCloseDaemonListenerUnlinksOnlyItsOwnedSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a2ui.sock")
	ln, perr := ipc.ListenUnix(path)
	if perr != nil {
		t.Fatal(perr)
	}
	closeDaemonListener(ln)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owned socket remains after listener close: %v", err)
	}
}
