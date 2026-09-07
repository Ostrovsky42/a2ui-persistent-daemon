package supervisor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"a2ui/agentclient"
)

const (
	PollInterval         = 250 * time.Millisecond
	StatusRequestTimeout = 2 * time.Second
)

var ErrStatusUnavailable = errors.New("daemon status unavailable")

type StatusSource interface {
	Status(context.Context) (agentclient.Status, error)
}

type PollerConfig struct {
	Source            StatusSource
	Controller        *Controller
	MaxStatusFailures int
	RequestTimeout    time.Duration
}

type Poller struct {
	config              PollerConfig
	consecutiveFailures int
}

func NewPoller(config PollerConfig) (*Poller, error) {
	if config.Source == nil || config.Controller == nil {
		return nil, errors.New("status source and controller are required")
	}
	if config.MaxStatusFailures <= 0 {
		config.MaxStatusFailures = 3
	}
	if config.RequestTimeout <= 0 {
		config.RequestTimeout = StatusRequestTimeout
	}
	return &Poller{config: config}, nil
}

func (p *Poller) Poll(ctx context.Context, now time.Time) error {
	requestCtx, cancel := context.WithTimeout(ctx, p.config.RequestTimeout)
	defer cancel()

	status, err := p.config.Source.Status(requestCtx)
	if err != nil {
		p.consecutiveFailures++
		if p.consecutiveFailures >= p.config.MaxStatusFailures {
			return fmt.Errorf("%w after %d consecutive failures: %v", ErrStatusUnavailable, p.consecutiveFailures, err)
		}
		return nil
	}

	p.consecutiveFailures = 0
	return p.config.Controller.Observe(ctx, status, now)
}

func (p *Poller) Run(ctx context.Context) error {
	if err := p.Poll(ctx, time.Now()); err != nil {
		return err
	}

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-ticker.C:
			if err := p.Poll(ctx, now); err != nil {
				return err
			}
		}
	}
}
