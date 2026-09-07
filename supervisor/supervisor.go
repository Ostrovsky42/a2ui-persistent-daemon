package supervisor

import (
	"context"
	"errors"
	"time"

	"a2ui/agentclient"
)

var (
	ErrDaemonReplaced         = errors.New("daemon instance replaced")
	ErrDaemonIdentityMismatch = errors.New("daemon identity mismatch")
)

type Identity struct {
	InstanceID string
	Socket     string
	Server     string
	Session    string
}

type ViewerSpec struct {
	Executable string
	Socket     string
	Preset     string
}

type Launcher interface {
	LaunchViewer(context.Context, ViewerSpec) error
}

type ControllerConfig struct {
	Identity      Identity
	Launcher      Launcher
	Viewer        ViewerSpec
	LaunchAllowed func() bool
	AttachTimeout time.Duration
}

type State struct {
	LastObservedGeneration  uint64
	LaunchPending           bool
	LaunchGeneration        uint64
	FailedThroughGeneration uint64
	LastFailure             string
}

type Controller struct {
	config ControllerConfig
	state  State
}

func NewController(config ControllerConfig) (*Controller, error) {
	if config.Launcher == nil {
		return nil, errors.New("launcher is required")
	}
	return &Controller{config: config}, nil
}

func (c *Controller) Observe(_ context.Context, status agentclient.Status, _ time.Time) error {
	c.state.LastObservedGeneration = status.Generation
	return nil
}

func (c *Controller) State() State {
	return c.state
}
