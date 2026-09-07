package supervisor

import (
	"context"
	"errors"
	"testing"
	"time"

	"a2ui/agentclient"
)

type fakeLauncher struct {
	calls []ViewerSpec
	errs  []error
}

func (f *fakeLauncher) LaunchViewer(_ context.Context, spec ViewerSpec) error {
	f.calls = append(f.calls, spec)
	if len(f.errs) == 0 {
		return nil
	}
	err := f.errs[0]
	f.errs = f.errs[1:]
	return err
}

func testIdentity() Identity {
	return Identity{
		InstanceID: "daemon-1",
		Socket:     "/tmp/a2ui-test/a2ui.sock",
		Server:     "http://127.0.0.1:18080",
		Session:    "default",
	}
}

func testStatus(generation uint64, pending, hasClient bool) agentclient.Status {
	id := testIdentity()
	return agentclient.Status{
		Session:        id.Session,
		Generation:     generation,
		PendingPublish: pending,
		HasClient:      hasClient,
		InstanceID:     id.InstanceID,
		Socket:         id.Socket,
		Server:         id.Server,
	}
}

func newTestController(t *testing.T, launcher Launcher) *Controller {
	t.Helper()
	controller, err := NewController(ControllerConfig{
		Identity:      testIdentity(),
		Launcher:      launcher,
		Viewer:        ViewerSpec{Executable: "/opt/a2ui/bin/a2ui", Socket: testIdentity().Socket, Preset: "dashboard"},
		LaunchAllowed: func() bool { return true },
		AttachTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	return controller
}

func observe(t *testing.T, c *Controller, status agentclient.Status, now time.Time) {
	t.Helper()
	if err := c.Observe(context.Background(), status, now); err != nil {
		t.Fatalf("Observe: %v", err)
	}
}

func TestViewerlessNewPublicationLaunchesExactlyOnce(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	now := time.Unix(100, 0)

	observe(t, controller, testStatus(10, false, false), now)
	observe(t, controller, testStatus(11, true, false), now.Add(time.Millisecond))

	if got := len(launcher.calls); got != 1 {
		t.Fatalf("LaunchViewer calls=%d, want 1", got)
	}
}

func TestViewerPresentSuppressesLaunch(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	now := time.Unix(200, 0)

	observe(t, controller, testStatus(20, false, true), now)
	observe(t, controller, testStatus(21, true, true), now.Add(time.Millisecond))

	if got := len(launcher.calls); got != 0 {
		t.Fatalf("LaunchViewer calls=%d, want 0", got)
	}
}

func TestRapidPublicationsCoalesceWhileAttachPending(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	now := time.Unix(300, 0)

	observe(t, controller, testStatus(30, false, false), now)
	observe(t, controller, testStatus(31, true, false), now.Add(time.Millisecond))
	observe(t, controller, testStatus(32, true, false), now.Add(2*time.Millisecond))
	observe(t, controller, testStatus(33, true, false), now.Add(3*time.Millisecond))

	if got := len(launcher.calls); got != 1 {
		t.Fatalf("LaunchViewer calls=%d, want 1", got)
	}
	if !controller.State().LaunchPending {
		t.Fatal("launch_pending=false, want true before authoritative attachment")
	}
}

func TestAuthoritativeAttachmentClearsPending(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	now := time.Unix(400, 0)

	observe(t, controller, testStatus(40, false, false), now)
	observe(t, controller, testStatus(41, true, false), now.Add(time.Millisecond))
	if !controller.State().LaunchPending {
		t.Fatal("launch_pending=false immediately after launch request")
	}
	observe(t, controller, testStatus(41, false, true), now.Add(2*time.Millisecond))

	if controller.State().LaunchPending {
		t.Fatal("launch_pending=true after has_client=true")
	}
}

func TestViewerCloseDoesNotReopenUntilLaterPendingGeneration(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	now := time.Unix(500, 0)

	observe(t, controller, testStatus(50, false, true), now)
	observe(t, controller, testStatus(50, false, false), now.Add(time.Millisecond))
	if got := len(launcher.calls); got != 0 {
		t.Fatalf("same-generation detach launched %d viewers, want 0", got)
	}
	observe(t, controller, testStatus(51, true, false), now.Add(2*time.Millisecond))
	if got := len(launcher.calls); got != 1 {
		t.Fatalf("later pending generation launched %d viewers, want 1", got)
	}
}

func TestImmediateLaunchFailureSuppressesSameGenerationAndLaterGenerationRetries(t *testing.T) {
	launchErr := errors.New("terminal unavailable")
	launcher := &fakeLauncher{errs: []error{launchErr, nil}}
	controller := newTestController(t, launcher)
	now := time.Unix(600, 0)

	observe(t, controller, testStatus(60, false, false), now)
	observe(t, controller, testStatus(61, true, false), now.Add(time.Millisecond))
	state := controller.State()
	if state.LaunchPending {
		t.Fatal("launch_pending=true after immediate launcher failure")
	}
	if state.FailedThroughGeneration < 61 {
		t.Fatalf("failed_through_generation=%d, want >=61", state.FailedThroughGeneration)
	}
	if state.LastFailure == "" {
		t.Fatal("last failure diagnostic is empty")
	}

	observe(t, controller, testStatus(61, true, false), now.Add(2*time.Millisecond))
	observe(t, controller, testStatus(61, true, false), now.Add(3*time.Millisecond))
	if got := len(launcher.calls); got != 1 {
		t.Fatalf("same generation launch calls=%d, want 1 total", got)
	}

	observe(t, controller, testStatus(62, true, false), now.Add(4*time.Millisecond))
	if got := len(launcher.calls); got != 2 {
		t.Fatalf("later generation launch calls=%d, want 2 total", got)
	}
}

func TestAttachTimeoutSuppressesSameGeneration(t *testing.T) {
	launcher := &fakeLauncher{}
	controller := newTestController(t, launcher)
	now := time.Unix(700, 0)

	observe(t, controller, testStatus(70, false, false), now)
	observe(t, controller, testStatus(71, true, false), now.Add(time.Millisecond))
	observe(t, controller, testStatus(71, true, false), now.Add(3*time.Second))

	state := controller.State()
	if state.LaunchPending {
		t.Fatal("launch_pending=true after attach timeout")
	}
	if state.FailedThroughGeneration < 71 {
		t.Fatalf("failed_through_generation=%d, want >=71", state.FailedThroughGeneration)
	}
	if state.LastFailure == "" {
		t.Fatal("attach timeout diagnostic is empty")
	}

	observe(t, controller, testStatus(71, true, false), now.Add(4*time.Second))
	if got := len(launcher.calls); got != 1 {
		t.Fatalf("same generation after timeout launch calls=%d, want 1 total", got)
	}
	observe(t, controller, testStatus(72, true, false), now.Add(5*time.Second))
	if got := len(launcher.calls); got != 2 {
		t.Fatalf("new generation after timeout launch calls=%d, want 2 total", got)
	}
}

func TestBootstrapLaunchesOnlyAlreadyPendingPublication(t *testing.T) {
	now := time.Unix(800, 0)

	pendingLauncher := &fakeLauncher{}
	pending := newTestController(t, pendingLauncher)
	observe(t, pending, testStatus(81, true, false), now)
	if got := len(pendingLauncher.calls); got != 1 {
		t.Fatalf("pending bootstrap launch calls=%d, want 1", got)
	}

	cleanLauncher := &fakeLauncher{}
	clean := newTestController(t, cleanLauncher)
	observe(t, clean, testStatus(81, false, false), now)
	if got := len(cleanLauncher.calls); got != 0 {
		t.Fatalf("non-pending bootstrap launch calls=%d, want 0", got)
	}
}

func TestDaemonReplacementOrIdentityMismatchStopsController(t *testing.T) {
	controller := newTestController(t, &fakeLauncher{})
	now := time.Unix(900, 0)
	observe(t, controller, testStatus(90, false, false), now)

	replaced := testStatus(91, true, false)
	replaced.InstanceID = "daemon-2"
	if err := controller.Observe(context.Background(), replaced, now.Add(time.Second)); !errors.Is(err, ErrDaemonReplaced) {
		t.Fatalf("instance replacement error=%v, want ErrDaemonReplaced", err)
	}

	controller = newTestController(t, &fakeLauncher{})
	observe(t, controller, testStatus(90, false, false), now)
	mismatched := testStatus(91, true, false)
	mismatched.Socket = "/tmp/other/a2ui.sock"
	if err := controller.Observe(context.Background(), mismatched, now.Add(time.Second)); !errors.Is(err, ErrDaemonIdentityMismatch) {
		t.Fatalf("socket mismatch error=%v, want ErrDaemonIdentityMismatch", err)
	}
}
