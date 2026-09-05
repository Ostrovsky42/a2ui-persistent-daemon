package engine

import (
    "reflect"
    "testing"

    "a2ui/protocol"
)

func TestPresentationSnapshotCarriesRenderGeneration(t *testing.T) {
    eng := New(protocol.DefaultLimits(), 16, nil)
    snap := eng.PresentationSnapshot()
    if _, ok := reflect.TypeOf(snap).FieldByName("RenderGeneration"); !ok {
        t.Fatal("PresentationSnapshot must expose RenderGeneration for monotonic remote snapshots")
    }
}
