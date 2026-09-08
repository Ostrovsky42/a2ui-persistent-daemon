//go:build unix

package main

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

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

func TestA2uidStatusIncludesStableRuntimeIdentityAndRestartChangesInstance(t *testing.T) {
	runtimeDir := t.TempDir()
	socket := filepath.Join(runtimeDir, "a2ui.sock")
	addr := reserveLoopbackAddress(t)

	first := startA2uidHelper(t, socket, addr)
	status1 := waitA2uidStatus(t, addr)
	status1Again := waitA2uidStatus(t, addr)
	stopA2uidHelper(t, first)

	instance1 := requiredStringField(t, status1, "instance_id")
	if got := requiredStringField(t, status1Again, "instance_id"); got != instance1 {
		t.Fatalf("instance_id changed within daemon lifetime: first=%q again=%q", instance1, got)
	}
	if got := requiredStringField(t, status1, "socket"); got != socket {
		t.Fatalf("socket=%q, want %q", got, socket)
	}
	wantServer := "http://" + addr
	if got := requiredStringField(t, status1, "server"); got != wantServer {
		t.Fatalf("server=%q, want %q", got, wantServer)
	}
	if got := requiredStringField(t, status1, "session"); got != "identity" {
		t.Fatalf("session=%q, want identity", got)
	}

	second := startA2uidHelper(t, socket, addr)
	status2 := waitA2uidStatus(t, addr)
	stopA2uidHelper(t, second)
	instance2 := requiredStringField(t, status2, "instance_id")
	if instance2 == instance1 {
		t.Fatalf("instance_id=%q survived daemon restart, want a new process-lifetime identity", instance2)
	}
}

func TestA2uidStatusIdentityHelper(t *testing.T) {
	if os.Getenv("A2UI_A2UID_HELPER") != "1" {
		return
	}
	os.Args = []string{
		"a2uid",
		"-socket", os.Getenv("A2UI_A2UID_SOCKET"),
		"-server", os.Getenv("A2UI_A2UID_SERVER"),
		"-session", "identity",
	}
	main()
}

func reserveLoopbackAddress(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve loopback address: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("release loopback address: %v", err)
	}
	return addr
}

func startA2uidHelper(t *testing.T, socket, addr string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestA2uidStatusIdentityHelper$")
	cmd.Env = append(os.Environ(),
		"A2UI_A2UID_HELPER=1",
		"A2UI_A2UID_SOCKET="+socket,
		"A2UI_A2UID_SERVER="+addr,
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start a2uid helper: %v", err)
	}
	return cmd
}

func stopA2uidHelper(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal a2uid helper: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); !ok || !exitErr.Success() {
			t.Fatalf("wait a2uid helper: %v", err)
		}
	}
}

func waitA2uidStatus(t *testing.T, addr string) map[string]any {
	t.Helper()
	client := &http.Client{Timeout: 150 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	url := "http://" + addr + "/status"
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			var status map[string]any
			decodeErr := json.NewDecoder(resp.Body).Decode(&status)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK && decodeErr == nil {
				return status
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("a2uid status did not become ready at %s", url)
	return nil
}

func requiredStringField(t *testing.T, status map[string]any, field string) string {
	t.Helper()
	value, ok := status[field].(string)
	if !ok || value == "" {
		t.Fatalf("status field %q missing or empty: %#v", field, status)
	}
	return value
}
