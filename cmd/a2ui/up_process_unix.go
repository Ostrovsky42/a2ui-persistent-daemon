//go:build unix

package main

import "os/exec"

type detachedProcessSpec struct {
	Path    string
	Args    []string
	LogPath string
	PIDPath string
}

type preparedDetachedCommand struct {
	cmd *exec.Cmd
}

func prepareDetachedCommand(spec detachedProcessSpec) (*preparedDetachedCommand, error) {
	return &preparedDetachedCommand{cmd: exec.Command(spec.Path, spec.Args...)}, nil
}

func (p *preparedDetachedCommand) close() error {
	return nil
}

func writePIDDiagnostic(string, int) error {
	return nil
}

func startDetached(detachedProcessSpec) error {
	return nil
}
