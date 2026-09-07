package localenv

import (
	"context"
	"errors"
	"time"

	"a2ui/agentclient"
)

var (
	ErrDaemonIncompatible = errors.New("daemon is incompatible with requested local environment")
	ErrDaemonReadiness    = errors.New("daemon did not become ready")
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

func Reconcile(context.Context, ReconcileConfig, ReconcileDependencies) (ReconcileResult, error) {
	return ReconcileResult{}, nil
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
