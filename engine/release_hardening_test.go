package engine

import (
	"reflect"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func TestPresentationSnapshotCarriesRenderGeneration(t *testing.T) {
	eng := New(protocol.DefaultLimits(), 16, nil)
	snap := eng.PresentationSnapshot()
	if _, ok := reflect.TypeOf(snap).FieldByName("RenderGeneration"); !ok {
		t.Fatal("PresentationSnapshot must expose RenderGeneration for monotonic remote snapshots")
	}
}
