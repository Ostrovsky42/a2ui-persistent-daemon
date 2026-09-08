//go:build unix

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

type detachedProcessSpec struct {
	Path    string
	Args    []string
	LogPath string
	PIDPath string
}

type preparedDetachedCommand struct {
	cmd   *exec.Cmd
	stdin *os.File
	log   *os.File
}

func prepareDetachedCommand(spec detachedProcessSpec) (*preparedDetachedCommand, error) {
	if spec.Path == "" || spec.LogPath == "" || spec.PIDPath == "" {
		return nil, fmt.Errorf("detached process path, log path, and PID path are required")
	}
	if err := secureRuntimeDir(filepath.Dir(spec.LogPath)); err != nil {
		return nil, err
	}
	if filepath.Dir(spec.PIDPath) != filepath.Dir(spec.LogPath) {
		if err := secureRuntimeDir(filepath.Dir(spec.PIDPath)); err != nil {
			return nil, err
		}
	}
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		return nil, fmt.Errorf("open detached stdin: %w", err)
	}
	log, err := os.OpenFile(spec.LogPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("open detached log: %w", err)
	}
	if err := log.Chmod(0o600); err != nil {
		_ = stdin.Close()
		_ = log.Close()
		return nil, fmt.Errorf("secure detached log: %w", err)
	}
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = stdin
	cmd.Stdout = log
	cmd.Stderr = log
	return &preparedDetachedCommand{cmd: cmd, stdin: stdin, log: log}, nil
}

func (p *preparedDetachedCommand) close() error {
	if p == nil {
		return nil
	}
	var first error
	if p.stdin != nil {
		if err := p.stdin.Close(); err != nil && first == nil {
			first = err
		}
	}
	if p.log != nil {
		if err := p.log.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func writePIDDiagnostic(path string, pid int) error {
	if path == "" || pid <= 0 {
		return fmt.Errorf("invalid PID diagnostic path or pid")
	}
	if err := secureRuntimeDir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".supervisor.pid.tmp-*")
	if err != nil {
		return fmt.Errorf("create PID diagnostic: %w", err)
	}
	tmpName := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("secure PID diagnostic: %w", err)
	}
	if _, err := fmt.Fprintf(tmp, "%d\n", pid); err != nil {
		return fmt.Errorf("write PID diagnostic: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync PID diagnostic: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close PID diagnostic: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace PID diagnostic: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("secure PID diagnostic path: %w", err)
	}
	committed = true
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open runtime directory: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync runtime directory: %w", err)
	}
	return nil
}

func startDetached(spec detachedProcessSpec) error {
	prepared, err := prepareDetachedCommand(spec)
	if err != nil {
		return err
	}
	if err := prepared.cmd.Start(); err != nil {
		_ = prepared.close()
		return fmt.Errorf("start detached process: %w", err)
	}
	pid := prepared.cmd.Process.Pid
	if err := writePIDDiagnostic(spec.PIDPath, pid); err != nil {
		_ = prepared.cmd.Process.Kill()
		_ = prepared.cmd.Process.Release()
		_ = prepared.close()
		return err
	}
	if err := prepared.cmd.Process.Release(); err != nil {
		_ = prepared.close()
		return fmt.Errorf("release detached process: %w", err)
	}
	return prepared.close()
}

func secureRuntimeDir(dir string) error {
	if dir == "" || !filepath.IsAbs(dir) {
		return fmt.Errorf("runtime directory must be absolute: %q", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create runtime directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("secure runtime directory: %w", err)
	}
	return nil
}
