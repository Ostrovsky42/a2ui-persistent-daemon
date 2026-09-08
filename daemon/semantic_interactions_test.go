package daemon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"a2ui/ipc"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
)

func TestSemanticActionInvokeDoesNotRequirePhysicalKey(t *testing.T) {
	actions := a2runtime.NewActionRegistry(time.Second, 1)
	called := make(chan json.RawMessage, 1)
	if err := actions.Register("approve", func(_ context.Context, args json.RawMessage) error {
		called <- append(json.RawMessage(nil), args...)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	d := New("semantic-action", protocol.DefaultLimits(), actions)
	if perr := d.Engine.Apply(protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "actions",
		Type:   protocol.NodeActions,
		Parent: "root",
		Props:  json.RawMessage(`{"items":[{"key":"y","label":"Approve","action":"approve","args":{"target":"prod"}}]}`),
	}); perr != nil {
		t.Fatal(perr)
	}

	perr := d.handleInteraction(context.Background(), &ipc.Interaction{
		Type:   ipc.InteractionActionInvoke,
		ID:     "actions",
		Action: "approve",
		Args:   json.RawMessage(`{"target":"prod"}`),
	})
	if perr != nil {
		t.Fatalf("semantic action invoke failed: %v", perr)
	}
	select {
	case got := <-called:
		var payload map[string]string
		if err := json.Unmarshal(got, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["target"] != "prod" {
			t.Fatalf("args=%s", got)
		}
	default:
		t.Fatal("registered action handler was not invoked")
	}
	ev, ok := d.Engine.NextEvent()
	if !ok || ev.Ev != "action_result" || ev.Action != "approve" {
		t.Fatalf("event=%+v ok=%v", ev, ok)
	}
}

func TestSemanticTableSelectUsesStableRowID(t *testing.T) {
	d := New("semantic-row", protocol.DefaultLimits(), nil)
	if perr := d.Engine.Apply(protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "jobs",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props:  json.RawMessage(`{"columns":[{"title":"Job"}],"rows":[["one"],["two"]],"row_ids":["job-1","job-2"],"selectable":true}`),
	}); perr != nil {
		t.Fatal(perr)
	}

	perr := d.handleInteraction(context.Background(), &ipc.Interaction{
		Type:  ipc.InteractionTableSelect,
		ID:    "jobs",
		RowID: "job-2",
	})
	if perr != nil {
		t.Fatalf("semantic row selection failed: %v", perr)
	}
	selection, ok := d.Engine.SelectedTableRow("jobs")
	if !ok || selection.Index != 1 || selection.RowID != "job-2" {
		t.Fatalf("selection=%+v ok=%v", selection, ok)
	}
}
