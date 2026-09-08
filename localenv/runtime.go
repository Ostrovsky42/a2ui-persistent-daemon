package localenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const DescriptorVersion = 1

var ErrDescriptorMismatch = errors.New("local environment descriptor mismatch")

type Descriptor struct {
	Version      int    `json:"version"`
	InstanceID   string `json:"instance_id"`
	Socket       string `json:"socket"`
	Server       string `json:"server"`
	Session      string `json:"session"`
	Preset       string `json:"preset"`
	ViewerPolicy string `json:"viewer_policy"`
}

type Paths struct {
	Dir            string
	Environment    string
	UpLock         string
	SupervisorLock string
	DaemonPID      string
	SupervisorPID  string
	DaemonLog      string
	SupervisorLog  string
}

func ResolvePaths(socket string) (Paths, error) {
	if socket == "" || !filepath.IsAbs(socket) {
		return Paths{}, fmt.Errorf("socket path must be absolute: %q", socket)
	}
	dir := filepath.Dir(filepath.Clean(socket))
	return Paths{
		Dir:            dir,
		Environment:    filepath.Join(dir, "environment.json"),
		UpLock:         filepath.Join(dir, "up.lock"),
		SupervisorLock: filepath.Join(dir, "supervisor.lock"),
		DaemonPID:      filepath.Join(dir, "a2uid.pid"),
		SupervisorPID:  filepath.Join(dir, "supervisor.pid"),
		DaemonLog:      filepath.Join(dir, "a2uid.log"),
		SupervisorLog:  filepath.Join(dir, "supervisor.log"),
	}, nil
}

func ensurePrivateDir(dir string) error {
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

func WriteDescriptor(paths Paths, descriptor Descriptor) error {
	if paths.Environment == "" || filepath.Dir(paths.Environment) != paths.Dir {
		return errors.New("invalid environment descriptor path")
	}
	if descriptor.Version == 0 {
		descriptor.Version = DescriptorVersion
	}
	if descriptor.Version != DescriptorVersion {
		return fmt.Errorf("unsupported local descriptor version %d", descriptor.Version)
	}
	if err := ensurePrivateDir(paths.Dir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(paths.Dir, ".environment.json.tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary descriptor: %w", err)
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
		return fmt.Errorf("secure temporary descriptor: %w", err)
	}
	if err := json.NewEncoder(tmp).Encode(descriptor); err != nil {
		return fmt.Errorf("encode descriptor: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("sync descriptor: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close descriptor: %w", err)
	}
	if err := os.Rename(tmpName, paths.Environment); err != nil {
		return fmt.Errorf("replace descriptor: %w", err)
	}
	if err := os.Chmod(paths.Environment, 0o600); err != nil {
		return fmt.Errorf("secure descriptor: %w", err)
	}
	committed = true

	dir, err := os.Open(paths.Dir)
	if err != nil {
		return fmt.Errorf("open runtime directory for sync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync runtime directory: %w", err)
	}
	return nil
}

func ReadDescriptor(paths Paths) (Descriptor, error) {
	file, err := os.Open(paths.Environment)
	if err != nil {
		return Descriptor{}, err
	}
	defer file.Close()

	var descriptor Descriptor
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("decode descriptor: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Descriptor{}, errors.New("descriptor contains trailing JSON")
		}
		return Descriptor{}, fmt.Errorf("decode descriptor trailing data: %w", err)
	}
	if descriptor.Version != DescriptorVersion {
		return Descriptor{}, fmt.Errorf("unsupported local descriptor version %d", descriptor.Version)
	}
	return descriptor, nil
}

func ValidateDescriptor(actual, expected Descriptor) error {
	if actual == expected {
		return nil
	}
	return fmt.Errorf(
		"%w: expected version=%d instance_id=%q socket=%q server=%q session=%q preset=%q viewer_policy=%q; got version=%d instance_id=%q socket=%q server=%q session=%q preset=%q viewer_policy=%q",
		ErrDescriptorMismatch,
		expected.Version,
		expected.InstanceID,
		expected.Socket,
		expected.Server,
		expected.Session,
		expected.Preset,
		expected.ViewerPolicy,
		actual.Version,
		actual.InstanceID,
		actual.Socket,
		actual.Server,
		actual.Session,
		actual.Preset,
		actual.ViewerPolicy,
	)
}
