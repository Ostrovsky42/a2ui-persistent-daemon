package localenv

import "errors"

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

func ResolvePaths(string) (Paths, error) {
	return Paths{}, nil
}

func WriteDescriptor(Paths, Descriptor) error {
	return nil
}

func ReadDescriptor(Paths) (Descriptor, error) {
	return Descriptor{}, nil
}

func ValidateDescriptor(Descriptor, Descriptor) error {
	return nil
}
