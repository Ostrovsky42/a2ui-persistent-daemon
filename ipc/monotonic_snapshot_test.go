//go:build unix

package ipc

import (
	"testing"

	"a2ui/engine"
)

func TestClientSnapshotCacheRejectsOlderRenderGeneration(t *testing.T) {
	c := &Client{
		snapshot: engine.PresentationSnapshot{RenderGeneration: 2},
		updates:  make(chan struct{}, 1),
	}
	if c.acceptSnapshot(engine.PresentationSnapshot{RenderGeneration: 1}) {
		t.Fatal("older snapshot must not replace a newer projection cache")
	}
	if got := c.Snapshot().RenderGeneration; got != 2 {
		t.Fatalf("render generation regressed to %d", got)
	}
	if !c.acceptSnapshot(engine.PresentationSnapshot{RenderGeneration: 3}) {
		t.Fatal("newer snapshot must be accepted")
	}
	if got := c.Snapshot().RenderGeneration; got != 3 {
		t.Fatalf("newer render generation not stored: %d", got)
	}
}
