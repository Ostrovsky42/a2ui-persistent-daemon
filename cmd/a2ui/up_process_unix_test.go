//go:build unix

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPrepareDetachedCommandHasIndependentSessionAndPrivateIO(t *testing.T) {
	dir := t.TempDir()
	spec := detachedProcessSpec{
		Path:    "/usr/bin/a2uid",
		Args:    []string{"--socket", filepath.Join(dir, "a2ui.sock"), "--server", "127.0.0.1:18080", "--session", "default"},
		LogPath: filepath.Join(dir, "a2uid.log"),
		PIDPath: filepath.Join(dir, "a2uid.pid"),
	}
	prepared, err := prepareDetachedCommand(spec)
	if err != nil {
		t.Fatalf("prepareDetachedCommand: %v", err)
	}
	defer prepared.close()

	wantArgs := append([]string{spec.Path}, spec.Args...)
	if !reflect.DeepEqual(prepared.cmd.Args, wantArgs) {
		t.Fatalf("argv=%q, want %q", prepared.cmd.Args, wantArgs)
	}
	if prepared.cmd.SysProcAttr == nil || !prepared.cmd.SysProcAttr.Setsid {
		t.Fatalf("SysProcAttr=%#v, want Setsid=true", prepared.cmd.SysProcAttr)
	}
	stdin, ok := prepared.cmd.Stdin.(*os.File)
	if !ok || stdin.Name() != os.DevNull {
		t.Fatalf("stdin=%T %#v, want %s", prepared.cmd.Stdin, prepared.cmd.Stdin, os.DevNull)
	}
	stdout, ok := prepared.cmd.Stdout.(*os.File)
	if !ok || stdout.Name() != spec.LogPath {
		t.Fatalf("stdout=%T %#v, want log %q", prepared.cmd.Stdout, prepared.cmd.Stdout, spec.LogPath)
	}
	stderr, ok := prepared.cmd.Stderr.(*os.File)
	if !ok || stderr != stdout {
		t.Fatalf("stderr=%T %#v, want same log file as stdout", prepared.cmd.Stderr, prepared.cmd.Stderr)
	}

	logInfo, err := os.Stat(spec.LogPath)
	if err != nil {
		t.Fatal(err)
	}
	if perm := logInfo.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log permissions=%#o, want 0600", perm)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Fatalf("runtime directory permissions=%#o, want 0700", perm)
	}
}

func TestWritePIDDiagnosticAtomicallyAndPrivately(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "supervisor.pid")
	if err := writePIDDiagnostic(path, 1234); err != nil {
		t.Fatalf("writePIDDiagnostic first: %v", err)
	}
	if err := writePIDDiagnostic(path, 5678); err != nil {
		t.Fatalf("writePIDDiagnostic replace: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "5678\n" {
		t.Fatalf("pid contents=%q, want 5678\\n", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pid permissions=%#o, want 0600", perm)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".supervisor.pid.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary PID files remain: %v", matches)
	}
}
