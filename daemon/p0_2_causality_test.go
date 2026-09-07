package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"a2ui/ipc"
	"a2ui/protocol"
)

func p02HandleOperation(t *testing.T, d *Daemon, seq uint64, op protocol.Operation) {
	t.Helper()
	op.V = protocol.Version
	op.Seq = int64(seq)
	payload, err := json.Marshal(op)
	if err != nil {
		t.Fatal(err)
	}
	_, perr := d.HandleEnvelope(protocol.Envelope{
		V:       protocol.Version,
		Session: d.Session.ID(),
		Kind:    protocol.KindOperation,
		Seq:     seq,
		Payload: payload,
	})
	if perr != nil {
		t.Fatalf("operation %d: %v", seq, perr)
	}
}

func TestP02UnreadSelectFromPreviousFrameDoesNotSatisfyNewScreenWait(t *testing.T) {
	d := New("p0-2-causality", protocol.DefaultLimits(), nil)
	if _, perr := d.Session.Negotiate(protocol.Hello{Versions: []int{protocol.Version}, Features: []string{"commit-barrier"}}); perr != nil {
		t.Fatal(perr)
	}

	p02HandleOperation(t, d, 1, protocol.Operation{
		Op:     protocol.OpUpsert,
		ID:     "workers",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props:  json.RawMessage(`{"selectable":true,"action":"worker.select","rows":[["image-worker"]],"row_ids":["image-worker"]}`),
	})
	p02HandleOperation(t, d, 2, protocol.Operation{Op: protocol.OpFocus, ID: "workers"})
	p02HandleOperation(t, d, 3, protocol.Operation{Op: protocol.OpCommit, Frame: "workers-A"})
	if perr := d.Engine.Publish(); perr != nil {
		t.Fatal(perr)
	}
	if ev, ok := d.Engine.NextEvent(); !ok || ev.Ev != "committed" || ev.Frame != "workers-A" {
		t.Fatalf("frame A publication event = %+v, ok=%v", ev, ok)
	}

	interaction := ipc.Interaction{Type: ipc.InteractionTableActivate, ID: "workers"}
	if ierr := d.handleInteraction(context.Background(), &interaction); ierr != nil {
		t.Fatal(ierr)
	}

	p02HandleOperation(t, d, 4, protocol.Operation{
		Op:    protocol.OpProps,
		ID:    "workers",
		Props: json.RawMessage(`{"rows":[["api-worker"]],"row_ids":["api-worker"]}`),
	})
	p02HandleOperation(t, d, 5, protocol.Operation{Op: protocol.OpCommit, Frame: "workers-B"})
	if perr := d.Engine.Publish(); perr != nil {
		t.Fatal(perr)
	}

	events := d.WaitEvents(context.Background(), 0)
	for _, ev := range events {
		if ev.Ev == "select" && ev.RowID == "image-worker" {
			t.Fatalf("stale select from frame A satisfied the post-frame-B wait: %+v", ev)
		}
	}
}
