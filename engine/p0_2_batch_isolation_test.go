package engine

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
)

func TestP02FailedBatchDoesNotAdvanceEventCursorOrEmitEvents(t *testing.T) {
	e := New(protocol.DefaultLimits(), protocol.DefaultLimits().MaxPendingEvents, nil)
	cursorBefore := e.EventCursor()
	docBefore := e.Document()

	perr := e.ApplyBatch([]protocol.Operation{
		{
			V:      protocol.Version,
			Seq:    1,
			Op:     protocol.OpUpsert,
			ID:     "panel",
			Type:   protocol.NodeBox,
			Parent: "root",
			Props:  json.RawMessage(`{"dir":"col"}`),
		},
		{
			V:     protocol.Version,
			Seq:   2,
			Op:    protocol.OpProps,
			ID:    "missing-node",
			Props: json.RawMessage(`{"gap":1}`),
		},
	})
	if perr == nil {
		t.Fatal("ApplyBatch succeeded, want semantic rejection")
	}
	if got := e.EventCursor(); got != cursorBefore {
		t.Fatalf("event cursor advanced from %d to %d on failed candidate batch", cursorBefore, got)
	}
	if ev, ok := e.NextEvent(); ok {
		t.Fatalf("failed candidate batch leaked event %+v", ev)
	}
	if got := e.Document(); got.Revision != docBefore.Revision || len(got.Nodes) != len(docBefore.Nodes) {
		t.Fatalf("failed candidate batch mutated document: before=%+v after=%+v", docBefore, got)
	}
}
