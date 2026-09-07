package supervisor

import (
	"context"
	"errors"
	"fmt"
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
	config         ControllerConfig
	state          State
	initialized    bool
	attachDeadline time.Time
}

func NewController(config ControllerConfig) (*Controller, error) {
	if config.Launcher == nil {
		return nil, errors.New("launcher is required")
	}
	if config.Identity.InstanceID == "" || config.Identity.Socket == "" || config.Identity.Server == "" || config.Identity.Session == "" {
		return nil, errors.New("complete daemon identity is required")
	}
	if config.LaunchAllowed == nil {
		config.LaunchAllowed = func() bool { return true }
	}
	if config.AttachTimeout <= 0 {
		config.AttachTimeout = 5 * time.Second
	}
	return &Controller{config: config}, nil
}

func (c *Controller) Observe(ctx context.Context, status agentclient.Status, now time.Time) error {
	if err := c.validateIdentity(status); err != nil {
		return err
	}

	if !c.initialized {
		c.initialized = true
		c.state.LastObservedGeneration = status.Generation
		if c.shouldLaunch(status, true) {
			c.requestLaunch(ctx, status.Generation, now)
		}
		return nil
	}

	previousGeneration := c.state.LastObservedGeneration
	if status.Generation > c.state.LastObservedGeneration {
		c.state.LastObservedGeneration = status.Generation
	}

	if c.state.LaunchPending {
		if status.HasClient {
			c.state.LaunchPending = false
			c.state.LaunchGeneration = 0
			c.state.LastFailure = ""
			c.attachDeadline = time.Time{}
			return nil
		}
		if !c.attachDeadline.IsZero() && !now.Before(c.attachDeadline) {
			c.failThrough(status.Generation, "viewer attach timeout")
		}
		return nil
	}

	if status.Generation <= previousGeneration {
		return nil
	}
	if c.shouldLaunch(status, false) {
		c.requestLaunch(ctx, status.Generation, now)
	}
	return nil
}

func (c *Controller) State() State {
	return c.state
}

func (c *Controller) validateIdentity(status agentclient.Status) error {
	if status.InstanceID != c.config.Identity.InstanceID {
		return fmt.Errorf("%w: expected instance %q, got %q", ErrDaemonReplaced, c.config.Identity.InstanceID, status.InstanceID)
	}
	if status.Socket != c.config.Identity.Socket || status.Server != c.config.Identity.Server || status.Session != c.config.Identity.Session {
		return fmt.Errorf(
			"%w: expected session=%q socket=%q server=%q, got session=%q socket=%q server=%q",
			ErrDaemonIdentityMismatch,
			c.config.Identity.Session,
			c.config.Identity.Socket,
			c.config.Identity.Server,
			status.Session,
			status.Socket,
			status.Server,
		)
	}
	return nil
}

func (c *Controller) shouldLaunch(status agentclient.Status, bootstrap bool) bool {
	if status.HasClient || !status.PendingPublish || status.Generation == 0 || c.state.LaunchPending {
		return false
	}
	if status.Generation <= c.state.FailedThroughGeneration {
		return false
	}
	if !bootstrap && status.Generation < c.state.LastObservedGeneration {
		return false
	}
	return c.config.LaunchAllowed()
}

func (c *Controller) requestLaunch(ctx context.Context, generation uint64, now time.Time) {
	c.state.LaunchPending = true
	c.state.LaunchGeneration = generation
	c.attachDeadline = now.Add(c.config.AttachTimeout)

	if err := c.config.Launcher.LaunchViewer(ctx, c.config.Viewer); err != nil {
		c.failThrough(generation, err.Error())
	}
}

func (c *Controller) failThrough(generation uint64, message string) {
	if c.state.LastObservedGeneration > generation {
		generation = c.state.LastObservedGeneration
	}
	if generation > c.state.FailedThroughGeneration {
		c.state.FailedThroughGeneration = generation
	}
	c.state.LaunchPending = false
	c.state.LaunchGeneration = 0
	c.state.LastFailure = message
	c.attachDeadline = time.Time{}
}
