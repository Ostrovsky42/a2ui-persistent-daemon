package supervisor

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"a2ui/agentclient"
)

type fakeStatusResult struct {
	status agentclient.Status
	err    error
}

type fakeStatusSource struct {
	results []fakeStatusResult
	calls   int
}

func (f *fakeStatusSource) Status(context.Context) (agentclient.Status, error) {
	f.calls++
	if len(f.results) == 0 {
		return agentclient.Status{}, errors.New("fake status exhausted")
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result.status, result.err
}

type deadlineCheckingStatusSource struct {
	status    agentclient.Status
	minBudget time.Duration
	observed  bool
}

func (s *deadlineCheckingStatusSource) Status(ctx context.Context) (agentclient.Status, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return agentclient.Status{}, errors.New("status request has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining < s.minBudget {
		return agentclient.Status{}, fmt.Errorf("status request budget %s is shorter than required %s", remaining, s.minBudget)
	}
	s.observed = true
	return s.status, nil
}

func TestSupervisorPollIntervalIsApprovedBoundedInterval(t *testing.T) {
	if PollInterval != 250*time.Millisecond {
		t.Fatalf("PollInterval=%s, want 250ms", PollInterval)
	}
}

func TestPollerRequestBudgetIsIndependentFromPollInterval(t *testing.T) {
	source := &deadlineCheckingStatusSource{
		status:    testStatus(95, false, false),
		minBudget: time.Second,
	}
	controller := newTestController(t, &fakeLauncher{})
	poller, err := NewPoller(PollerConfig{Source: source, Controller: controller, MaxStatusFailures: 1})
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	if err := poller.Poll(context.Background(), time.Unix(950, 0)); err != nil {
		t.Fatalf("Poll returned %v, want request deadline substantially larger than %s poll cadence", err, PollInterval)
	}
	if !source.observed {
		t.Fatal("status source did not observe an independent request budget")
	}
}

func TestPollerUsesStatusSourceAndController(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	source := &fakeStatusSource{results: []fakeStatusResult{
		{status: testStatus(100, false, false)},
		{status: testStatus(101, true, false)},
	}}
	poller, err := NewPoller(PollerConfig{Source: source, Controller: controller, MaxStatusFailures: 3})
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	now := time.Unix(1000, 0)
	if err := poller.Poll(context.Background(), now); err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	if err := poller.Poll(context.Background(), now.Add(PollInterval)); err != nil {
		t.Fatalf("second Poll: %v", err)
	}
	if source.calls != 2 {
		t.Fatalf("StatusSource calls=%d, want 2", source.calls)
	}
	if got := len(launcher.calls); got != 1 {
		t.Fatalf("LaunchViewer calls=%d, want 1", got)
	}
}

func TestPollerExitsOnProvenDaemonReplacement(t *testing.T) {
	controller := newTestController(t, &fakeLauncher{})
	replaced := testStatus(111, true, false)
	replaced.InstanceID = "replacement"
	source := &fakeStatusSource{results: []fakeStatusResult{
		{status: testStatus(110, false, false)},
		{status: replaced},
	}}
	poller, err := NewPoller(PollerConfig{Source: source, Controller: controller, MaxStatusFailures: 3})
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	now := time.Unix(1100, 0)
	if err := poller.Poll(context.Background(), now); err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	if err := poller.Poll(context.Background(), now.Add(PollInterval)); !errors.Is(err, ErrDaemonReplaced) {
		t.Fatalf("replacement Poll error=%v, want ErrDaemonReplaced", err)
	}
}

func TestPollerAllowsBoundedTransientStatusFailuresThenStops(t *testing.T) {
	statusErr := errors.New("connection refused")
	source := &fakeStatusSource{results: []fakeStatusResult{
		{err: statusErr},
		{err: statusErr},
		{err: statusErr},
	}}
	controller := newTestController(t, &fakeLauncher{})
	poller, err := NewPoller(PollerConfig{Source: source, Controller: controller, MaxStatusFailures: 3})
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	now := time.Unix(1200, 0)
	if err := poller.Poll(context.Background(), now); err != nil {
		t.Fatalf("first transient failure returned %v", err)
	}
	if err := poller.Poll(context.Background(), now.Add(PollInterval)); err != nil {
		t.Fatalf("second transient failure returned %v", err)
	}
	if err := poller.Poll(context.Background(), now.Add(2*PollInterval)); !errors.Is(err, ErrStatusUnavailable) {
		t.Fatalf("third transient failure error=%v, want ErrStatusUnavailable", err)
	}
}

func TestSuccessfulStatusReadResetsFailureGrace(t *testing.T) {
	statusErr := errors.New("temporary failure")
	source := &fakeStatusSource{results: []fakeStatusResult{
		{err: statusErr},
		{status: testStatus(120, false, false)},
		{err: statusErr},
		{err: statusErr},
	}}
	controller := newTestController(t, &fakeLauncher{})
	poller, err := NewPoller(PollerConfig{Source: source, Controller: controller, MaxStatusFailures: 3})
	if err != nil {
		t.Fatalf("NewPoller: %v", err)
	}

	now := time.Unix(1300, 0)
	for i := 0; i < 4; i++ {
		if err := poller.Poll(context.Background(), now.Add(time.Duration(i)*PollInterval)); err != nil {
			t.Fatalf("Poll %d returned %v, want failure grace reset after successful read", i+1, err)
		}
	}
}
