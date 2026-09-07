package supervisor

import (
	"context"
	"errors"
	"time"

	"a2ui/agentclient"
)

const PollInterval = 250 * time.Millisecond

var ErrStatusUnavailable = errors.New("daemon status unavailable")

type StatusSource interface {
	Status(context.Context) (agentclient.Status, error)
}

type PollerConfig struct {
	Source            StatusSource
	Controller        *Controller
	MaxStatusFailures int
}

type Poller struct {
	config PollerConfig
}

func NewPoller(config PollerConfig) (*Poller, error) {
	if config.Source == nil || config.Controller == nil {
		return nil, errors.New("status source and controller are required")
	}
	return &Poller{config: config}, nil
}

func (p *Poller) Poll(context.Context, time.Time) error {
	return nil
}
