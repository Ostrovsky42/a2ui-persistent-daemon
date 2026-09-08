package localenv

import (
	"context"
	"errors"
	"testing"

	"a2ui/agentclient"
)

func TestStartedDaemonMayExposeSocketBeforeStatusReady(t *testing.T) {
	cfg := reconcileConfig(t)
	runtime := &fakeRuntime{statusErr: errors.New("connection refused")}
	deps := reconcileDeps(runtime)
	deps.StartDaemon = func(_ context.Context, _ Descriptor, _ Paths) error {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		runtime.daemonStarts++
		runtime.socketLive = true
		runtime.serverLive = true
		return nil
	}
	statusReadsAfterStart := 0
	deps.Status = func(context.Context, string, string) (agentclient.Status, error) {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		if runtime.daemonStarts == 0 {
			return agentclient.Status{}, errors.New("connection refused")
		}
		statusReadsAfterStart++
		if statusReadsAfterStart == 1 {
			return agentclient.Status{}, errors.New("connection refused")
		}
		return healthyStatus("started-daemon", cfg.Desired.Socket, cfg.Desired.Server, cfg.Desired.Session), nil
	}

	result, err := Reconcile(context.Background(), cfg, deps)
	if err != nil {
		t.Fatalf("Reconcile during normal staggered startup: %v", err)
	}
	if result.Descriptor.InstanceID != "started-daemon" {
		t.Fatalf("result=%#v", result)
	}
	daemons, supervisors := runtime.counts()
	if daemons != 1 || supervisors != 1 {
		t.Fatalf("starts daemon=%d supervisor=%d, want 1/1", daemons, supervisors)
	}
}
