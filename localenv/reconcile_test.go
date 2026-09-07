package localenv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"a2ui/agentclient"
)

type fakeRuntime struct {
	mu                sync.Mutex
	socketLive        bool
	serverLive        bool
	supervisorRunning bool
	status            agentclient.Status
	statusErr         error
	daemonStarts      int
	supervisorStarts  int
}

func healthyStatus(instance, socket, server, session string) agentclient.Status {
	return agentclient.Status{
		Session:    session,
		InstanceID: instance,
		Socket:     socket,
		Server:     server,
	}
}

func (f *fakeRuntime) socketProbe(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.socketLive, nil
}

func (f *fakeRuntime) serverProbe(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.serverLive, nil
}

func (f *fakeRuntime) statusProbe(context.Context, string, string) (agentclient.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status, f.statusErr
}

func (f *fakeRuntime) supervisorProbe(context.Context, string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.supervisorRunning, nil
}

func (f *fakeRuntime) startDaemon(_ context.Context, descriptor Descriptor, _ Paths) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.daemonStarts++
	f.socketLive = true
	f.serverLive = true
	f.statusErr = nil
	f.status = healthyStatus("started-daemon", descriptor.Socket, descriptor.Server, descriptor.Session)
	return nil
}

func (f *fakeRuntime) startSupervisor(context.Context, Descriptor, Paths) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.supervisorStarts++
	f.supervisorRunning = true
	return nil
}

func (f *fakeRuntime) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.daemonStarts, f.supervisorStarts
}

func reconcileConfig(t *testing.T) ReconcileConfig {
	t.Helper()
	dir := t.TempDir()
	socket := filepath.Join(dir, "a2ui.sock")
	paths, err := ResolvePaths(socket)
	if err != nil {
		t.Fatal(err)
	}
	return ReconcileConfig{
		Paths: paths,
		Desired: Descriptor{
			Version:      DescriptorVersion,
			Socket:       socket,
			Server:       "http://127.0.0.1:18080",
			Session:      "default",
			Preset:       "dashboard",
			ViewerPolicy: "auto",
		},
		MaxReadinessPolls: 3,
	}
}

func reconcileDeps(runtime *fakeRuntime) ReconcileDependencies {
	return ReconcileDependencies{
		SocketLive:        runtime.socketProbe,
		ServerLive:        runtime.serverProbe,
		Status:            runtime.statusProbe,
		SupervisorRunning: runtime.supervisorProbe,
		StartDaemon:       runtime.startDaemon,
		StartSupervisor:   runtime.startSupervisor,
		WaitPoll:          func(context.Context) error { return nil },
	}
}

func TestU1FirstUpStartsOneDaemonAndOneSupervisor(t *testing.T) {
	cfg := reconcileConfig(t)
	runtime := &fakeRuntime{statusErr: errors.New("connection refused")}
	result, err := Reconcile(context.Background(), cfg, reconcileDeps(runtime))
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	daemons, supervisors := runtime.counts()
	if daemons != 1 || supervisors != 1 {
		t.Fatalf("starts daemon=%d supervisor=%d, want 1/1", daemons, supervisors)
	}
	if result.DaemonReused || result.SupervisorReused {
		t.Fatalf("first up incorrectly reported reuse: %#v", result)
	}
	if result.Descriptor.InstanceID != "started-daemon" {
		t.Fatalf("result descriptor=%#v", result.Descriptor)
	}
}

func TestU2SecondUpReusesHealthyEnvironment(t *testing.T) {
	cfg := reconcileConfig(t)
	runtime := &fakeRuntime{
		socketLive:        true,
		serverLive:        true,
		supervisorRunning: true,
		status:            healthyStatus("existing-daemon", cfg.Desired.Socket, cfg.Desired.Server, cfg.Desired.Session),
	}
	result, err := Reconcile(context.Background(), cfg, reconcileDeps(runtime))
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	daemons, supervisors := runtime.counts()
	if daemons != 0 || supervisors != 0 {
		t.Fatalf("starts daemon=%d supervisor=%d, want 0/0", daemons, supervisors)
	}
	if !result.DaemonReused || !result.SupervisorReused {
		t.Fatalf("reuse result=%#v, want daemon+supervisor reused", result)
	}
}

func TestU3ConcurrentUpConvergesToOneDaemonAndOneSupervisor(t *testing.T) {
	cfg := reconcileConfig(t)
	runtime := &fakeRuntime{statusErr: errors.New("connection refused")}
	deps := reconcileDeps(runtime)

	start := make(chan struct{})
	results := make(chan ReconcileResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := Reconcile(context.Background(), cfg, deps)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Reconcile: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("successful results=%d, want 2", len(results))
	}
	daemons, supervisors := runtime.counts()
	if daemons != 1 || supervisors != 1 {
		t.Fatalf("concurrent starts daemon=%d supervisor=%d, want 1/1", daemons, supervisors)
	}
}

func TestU4HealthyDaemonMissingSupervisorStartsSupervisorOnly(t *testing.T) {
	cfg := reconcileConfig(t)
	runtime := &fakeRuntime{
		socketLive: true,
		serverLive: true,
		status:     healthyStatus("existing-daemon", cfg.Desired.Socket, cfg.Desired.Server, cfg.Desired.Session),
	}
	result, err := Reconcile(context.Background(), cfg, reconcileDeps(runtime))
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	daemons, supervisors := runtime.counts()
	if daemons != 0 || supervisors != 1 {
		t.Fatalf("starts daemon=%d supervisor=%d, want 0/1", daemons, supervisors)
	}
	if !result.DaemonReused || result.SupervisorReused {
		t.Fatalf("result=%#v, want daemon reused and supervisor started", result)
	}
}

func TestU5IncompatibleDaemonStatesRefuseSecondDaemon(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*fakeRuntime, ReconcileConfig)
		wantErr error
	}{
		{
			name: "socket live status unreachable",
			mutate: func(r *fakeRuntime, _ ReconcileConfig) {
				r.socketLive = true
				r.serverLive = true
				r.statusErr = errors.New("status unavailable")
			},
			wantErr: ErrDaemonIncompatible,
		},
		{
			name: "wrong session",
			mutate: func(r *fakeRuntime, cfg ReconcileConfig) {
				r.socketLive = true
				r.serverLive = true
				r.status = healthyStatus("instance", cfg.Desired.Socket, cfg.Desired.Server, "other")
			},
			wantErr: ErrDaemonIncompatible,
		},
		{
			name: "socket status identity mismatch",
			mutate: func(r *fakeRuntime, cfg ReconcileConfig) {
				r.socketLive = true
				r.serverLive = true
				r.status = healthyStatus("instance", cfg.Desired.Socket+".other", cfg.Desired.Server, cfg.Desired.Session)
			},
			wantErr: ErrDaemonIncompatible,
		},
		{
			name: "unrelated HTTP endpoint",
			mutate: func(r *fakeRuntime, _ ReconcileConfig) {
				r.socketLive = false
				r.serverLive = true
				r.statusErr = errors.New("invalid status JSON")
			},
			wantErr: ErrDaemonIncompatible,
		},
		{
			name: "old daemon missing runtime identity",
			mutate: func(r *fakeRuntime, cfg ReconcileConfig) {
				r.socketLive = true
				r.serverLive = true
				r.status = agentclient.Status{Session: cfg.Desired.Session}
			},
			wantErr: ErrDaemonIncompatible,
		},
		{
			name: "status live expected socket absent",
			mutate: func(r *fakeRuntime, cfg ReconcileConfig) {
				r.socketLive = false
				r.serverLive = true
				r.status = healthyStatus("instance", cfg.Desired.Socket, cfg.Desired.Server, cfg.Desired.Session)
			},
			wantErr: ErrDaemonIncompatible,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := reconcileConfig(t)
			runtime := &fakeRuntime{}
			tt.mutate(runtime, cfg)
			_, err := Reconcile(context.Background(), cfg, reconcileDeps(runtime))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Reconcile error=%v, want %v", err, tt.wantErr)
			}
			daemons, _ := runtime.counts()
			if daemons != 0 {
				t.Fatalf("incompatible state started %d daemons, want 0", daemons)
			}
		})
	}
}

func TestU6StaleMetadataRecoversWhenNoAuthoritativeOwnerExists(t *testing.T) {
	cfg := reconcileConfig(t)
	stale := cfg.Desired
	stale.InstanceID = "stale-instance"
	if err := WriteDescriptor(cfg.Paths, stale); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Paths.DaemonPID, []byte("999999\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg.Paths.SupervisorPID, []byte("999998\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runtime := &fakeRuntime{statusErr: errors.New("connection refused")}
	result, err := Reconcile(context.Background(), cfg, reconcileDeps(runtime))
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	got, err := ReadDescriptor(cfg.Paths)
	if err != nil {
		t.Fatalf("ReadDescriptor: %v", err)
	}
	if got.InstanceID != "started-daemon" || result.Descriptor.InstanceID != "started-daemon" {
		t.Fatalf("stale descriptor not replaced: file=%#v result=%#v", got, result.Descriptor)
	}
	daemons, supervisors := runtime.counts()
	if daemons != 1 || supervisors != 1 {
		t.Fatalf("recovery starts daemon=%d supervisor=%d, want 1/1", daemons, supervisors)
	}
}

func TestReconcileReadinessIsBounded(t *testing.T) {
	cfg := reconcileConfig(t)
	cfg.MaxReadinessPolls = 2
	runtime := &fakeRuntime{statusErr: errors.New("connection refused")}
	deps := reconcileDeps(runtime)
	deps.StartDaemon = func(context.Context, Descriptor, Paths) error {
		runtime.mu.Lock()
		runtime.daemonStarts++
		runtime.mu.Unlock()
		return nil
	}
	polls := 0
	deps.WaitPoll = func(context.Context) error {
		polls++
		return nil
	}
	_, err := Reconcile(context.Background(), cfg, deps)
	if !errors.Is(err, ErrDaemonReadiness) {
		t.Fatalf("Reconcile error=%v, want ErrDaemonReadiness", err)
	}
	if polls > cfg.MaxReadinessPolls {
		t.Fatalf("readiness waits=%d, max=%d", polls, cfg.MaxReadinessPolls)
	}
}

func TestDefaultWaitPollIsBoundedAndContextAware(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if err := waitPoll(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waitPoll error=%v, want context.Canceled", err)
	}
	if time.Since(started) > 100*time.Millisecond {
		t.Fatalf("canceled waitPoll blocked too long")
	}
}
