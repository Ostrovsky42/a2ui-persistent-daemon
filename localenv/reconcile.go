package localenv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"a2ui/agentclient"
)

var (
	ErrDaemonIncompatible  = errors.New("daemon is incompatible with requested local environment")
	ErrDaemonReadiness     = errors.New("daemon did not become ready")
	ErrSupervisorReadiness = errors.New("supervisor did not become ready")
)

type ReconcileConfig struct {
	Paths             Paths
	Desired           Descriptor
	MaxReadinessPolls int
}

type ReconcileDependencies struct {
	AcquireUpLock     func(string) (*Lock, error)
	SocketLive        func(context.Context, string) (bool, error)
	ServerLive        func(context.Context, string) (bool, error)
	Status            func(context.Context, string, string) (agentclient.Status, error)
	SupervisorRunning func(context.Context, string) (bool, error)
	StartDaemon       func(context.Context, Descriptor, Paths) error
	StartSupervisor   func(context.Context, Descriptor, Paths) error
	WaitPoll          func(context.Context) error
}

type ReconcileResult struct {
	Descriptor       Descriptor
	DaemonReused     bool
	SupervisorReused bool
}

type daemonObservation struct {
	status  agentclient.Status
	healthy bool
	absent  bool
}

func Reconcile(ctx context.Context, config ReconcileConfig, deps ReconcileDependencies) (ReconcileResult, error) {
	if err := validateReconcileConfig(config, deps); err != nil {
		return ReconcileResult{}, err
	}
	if config.Desired.Version == 0 {
		config.Desired.Version = DescriptorVersion
	}
	if config.MaxReadinessPolls <= 0 {
		config.MaxReadinessPolls = 60
	}
	acquire := deps.AcquireUpLock
	if acquire == nil {
		acquire = AcquireUpLock
	}
	wait := deps.WaitPoll
	if wait == nil {
		wait = waitPoll
	}

	lock, err := acquire(config.Paths.UpLock)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("acquire up.lock: %w", err)
	}
	defer lock.Release()

	observation, err := observeDaemon(ctx, config.Desired, deps)
	if err != nil {
		return ReconcileResult{}, err
	}
	daemonReused := observation.healthy
	if observation.absent {
		if err := deps.StartDaemon(ctx, config.Desired, config.Paths); err != nil {
			return ReconcileResult{}, fmt.Errorf("start daemon: %w", err)
		}
		observation, err = waitForDaemon(ctx, config, deps, wait)
		if err != nil {
			return ReconcileResult{}, err
		}
	}
	if !observation.healthy {
		return ReconcileResult{}, ErrDaemonReadiness
	}

	live := config.Desired
	live.Version = DescriptorVersion
	live.InstanceID = observation.status.InstanceID
	if err := WriteDescriptor(config.Paths, live); err != nil {
		return ReconcileResult{}, fmt.Errorf("write runtime descriptor: %w", err)
	}

	supervisorRunning, err := deps.SupervisorRunning(ctx, config.Paths.SupervisorLock)
	if err != nil {
		return ReconcileResult{}, fmt.Errorf("probe supervisor: %w", err)
	}
	supervisorReused := supervisorRunning
	if !supervisorRunning {
		if err := deps.StartSupervisor(ctx, live, config.Paths); err != nil {
			return ReconcileResult{}, fmt.Errorf("start supervisor: %w", err)
		}
		if err := waitForSupervisor(ctx, config, deps, wait); err != nil {
			return ReconcileResult{}, err
		}
	}

	return ReconcileResult{
		Descriptor:       live,
		DaemonReused:     daemonReused,
		SupervisorReused: supervisorReused,
	}, nil
}

func validateReconcileConfig(config ReconcileConfig, deps ReconcileDependencies) error {
	if config.Desired.Socket == "" || config.Desired.Server == "" || config.Desired.Session == "" {
		return errors.New("socket, server, and session are required")
	}
	if config.Paths.UpLock == "" || config.Paths.Environment == "" || config.Paths.SupervisorLock == "" {
		return errors.New("runtime paths are required")
	}
	if deps.SocketLive == nil || deps.ServerLive == nil || deps.Status == nil || deps.SupervisorRunning == nil {
		return errors.New("runtime probes are required")
	}
	if deps.StartDaemon == nil || deps.StartSupervisor == nil {
		return errors.New("process starters are required")
	}
	return nil
}

func observeDaemon(ctx context.Context, desired Descriptor, deps ReconcileDependencies) (daemonObservation, error) {
	socketLive, err := deps.SocketLive(ctx, desired.Socket)
	if err != nil {
		return daemonObservation{}, fmt.Errorf("probe daemon socket: %w", err)
	}
	status, statusErr := deps.Status(ctx, desired.Server, desired.Session)
	if statusErr == nil {
		if !socketLive {
			return daemonObservation{}, fmt.Errorf("%w: HTTP status is live while expected Unix socket %q is absent", ErrDaemonIncompatible, desired.Socket)
		}
		if err := validateLiveStatus(status, desired); err != nil {
			return daemonObservation{}, err
		}
		return daemonObservation{status: status, healthy: true}, nil
	}
	if err := ctx.Err(); err != nil {
		return daemonObservation{}, err
	}
	if socketLive {
		return daemonObservation{}, fmt.Errorf("%w: Unix socket %q is live but /status failed: %v", ErrDaemonIncompatible, desired.Socket, statusErr)
	}
	serverLive, err := deps.ServerLive(ctx, desired.Server)
	if err != nil {
		return daemonObservation{}, fmt.Errorf("probe daemon server: %w", err)
	}
	if serverLive {
		return daemonObservation{}, fmt.Errorf("%w: HTTP endpoint %q is occupied but does not expose compatible A2UI status: %v", ErrDaemonIncompatible, desired.Server, statusErr)
	}
	return daemonObservation{absent: true}, nil
}

func observeStartedDaemon(ctx context.Context, desired Descriptor, deps ReconcileDependencies) (daemonObservation, error) {
	socketLive, err := deps.SocketLive(ctx, desired.Socket)
	if err != nil {
		return daemonObservation{}, fmt.Errorf("probe started daemon socket: %w", err)
	}
	status, statusErr := deps.Status(ctx, desired.Server, desired.Session)
	if statusErr != nil {
		if err := ctx.Err(); err != nil {
			return daemonObservation{}, err
		}
		return daemonObservation{}, nil
	}
	if err := validateLiveStatus(status, desired); err != nil {
		return daemonObservation{}, err
	}
	if !socketLive {
		return daemonObservation{}, nil
	}
	return daemonObservation{status: status, healthy: true}, nil
}

func validateLiveStatus(status agentclient.Status, desired Descriptor) error {
	if status.InstanceID == "" || status.Socket == "" || status.Server == "" || status.Session == "" {
		return fmt.Errorf("%w: live daemon is missing required runtime identity", ErrDaemonIncompatible)
	}
	if status.Socket != desired.Socket {
		return fmt.Errorf("%w: daemon reports socket %q, expected %q", ErrDaemonIncompatible, status.Socket, desired.Socket)
	}
	if status.Server != desired.Server {
		return fmt.Errorf("%w: daemon reports server %q, expected %q", ErrDaemonIncompatible, status.Server, desired.Server)
	}
	if status.Session != desired.Session {
		return fmt.Errorf("%w: daemon reports session %q, expected %q", ErrDaemonIncompatible, status.Session, desired.Session)
	}
	return nil
}

func waitForDaemon(ctx context.Context, config ReconcileConfig, deps ReconcileDependencies, wait func(context.Context) error) (daemonObservation, error) {
	for attempt := 0; attempt < config.MaxReadinessPolls; attempt++ {
		observation, err := observeStartedDaemon(ctx, config.Desired, deps)
		if err != nil {
			return daemonObservation{}, err
		}
		if observation.healthy {
			return observation, nil
		}
		if attempt+1 < config.MaxReadinessPolls {
			if err := wait(ctx); err != nil {
				return daemonObservation{}, err
			}
		}
	}
	return daemonObservation{}, ErrDaemonReadiness
}

func waitForSupervisor(ctx context.Context, config ReconcileConfig, deps ReconcileDependencies, wait func(context.Context) error) error {
	for attempt := 0; attempt < config.MaxReadinessPolls; attempt++ {
		running, err := deps.SupervisorRunning(ctx, config.Paths.SupervisorLock)
		if err != nil {
			return fmt.Errorf("probe supervisor: %w", err)
		}
		if running {
			return nil
		}
		if attempt+1 < config.MaxReadinessPolls {
			if err := wait(ctx); err != nil {
				return err
			}
		}
	}
	return ErrSupervisorReadiness
}

func waitPoll(ctx context.Context) error {
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
