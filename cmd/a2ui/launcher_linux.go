//go:build linux

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"a2ui/supervisor"
)

type linuxTerminalBackend struct {
	name   string
	prefix []string
}

var linuxTerminalBackends = []linuxTerminalBackend{
	{name: "xdg-terminal-exec", prefix: []string{"--"}},
	{name: "gnome-terminal", prefix: []string{"--"}},
	{name: "kitty"},
	{name: "alacritty", prefix: []string{"-e"}},
	{name: "konsole", prefix: []string{"-e"}},
}

func launchViewerInTerminal(_ context.Context, spec supervisor.ViewerSpec, logPath string) error {
	path, args, err := resolveLinuxTerminalCommand(spec)
	if err != nil {
		return err
	}

	log, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open supervisor log: %w", err)
	}
	defer log.Close()
	if err := log.Chmod(0o600); err != nil {
		return fmt.Errorf("secure supervisor log: %w", err)
	}

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("open terminal launcher stdin: %w", err)
	}
	defer devNull.Close()

	cmd := exec.Command(path, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin = devNull
	cmd.Stdout = log
	cmd.Stderr = log
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start terminal viewer via %s: %w", path, err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release terminal launcher process: %w", err)
	}
	return nil
}

func resolveLinuxTerminalCommand(spec supervisor.ViewerSpec) (string, []string, error) {
	if spec.Executable == "" || spec.Socket == "" || spec.Preset == "" {
		return "", nil, fmt.Errorf("viewer executable, socket, and preset are required")
	}

	viewerArgs := []string{spec.Executable, "-socket", spec.Socket, "-preset", spec.Preset}
	for _, backend := range linuxTerminalBackends {
		path, err := exec.LookPath(backend.name)
		if err != nil {
			continue
		}
		args := make([]string, 0, len(backend.prefix)+len(viewerArgs))
		args = append(args, backend.prefix...)
		args = append(args, viewerArgs...)
		return path, args, nil
	}

	names := make([]string, 0, len(linuxTerminalBackends))
	for _, backend := range linuxTerminalBackends {
		names = append(names, backend.name)
	}
	return "", nil, fmt.Errorf("no supported terminal emulator found; tried %s", strings.Join(names, ", "))
}
