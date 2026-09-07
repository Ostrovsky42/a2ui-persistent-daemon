package daemon

import (
	"testing"

	"a2ui/protocol"
)

// TestViewerlessPublicationSignalsCoalesce characterizes the current daemon
// boundary for a future launcher: it can observe no attached viewer, but it
// only retains one pending renderer update and has no terminal-launch owner.
func TestViewerlessPublicationSignalsCoalesce(t *testing.T) {
	d := New("viewerless", protocol.DefaultLimits(), nil)
	if d.HasActiveClient() {
		t.Fatal("new daemon unexpectedly has an interactive viewer")
	}
	for range 3 {
		d.signalSnapshot()
	}
	if got := len(d.updates); got != 1 {
		t.Fatalf("viewerless rapid publications retained %d update signals, want one", got)
	}
	if d.HasActiveClient() {
		t.Fatal("snapshot signals must not fabricate a viewer attachment")
	}
}
