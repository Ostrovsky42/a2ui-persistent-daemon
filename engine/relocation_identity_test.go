package engine

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
)

func applyRelocationOp(t *testing.T, e *Engine, op protocol.Operation) {
	t.Helper()
	if err := e.Apply(op); err != nil {
		t.Fatalf("apply failed: %v", err)
	}
}

func TestRelocatingSubtreePreservesInputValueAndFocus(t *testing.T) {
	e := New(protocol.DefaultLimits(), 32, NewNoopActions())
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "left", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{}`)})
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "right", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{}`)})
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "panel", Type: protocol.NodeBox, Parent: "left", Props: json.RawMessage(`{}`)})
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 4, Op: protocol.OpUpsert, ID: "query", Type: protocol.NodeInput, Parent: "panel", Props: json.RawMessage(`{"value":"seed"}`)})

	if err := e.SetInput("query", "typed by user"); err != nil {
		t.Fatal(err)
	}
	if err := e.Focus("query"); err != nil {
		t.Fatal(err)
	}

	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 5, Op: protocol.OpUpsert, ID: "panel", Type: protocol.NodeBox, Parent: "right", Props: json.RawMessage(`{}`)})

	if got := e.InputValue("query"); got != "typed by user" {
		t.Fatalf("input value=%q want=%q", got, "typed by user")
	}
	if got := e.FocusedID(); got != "query" {
		t.Fatalf("focused=%q want=query", got)
	}
	if got := e.Document().Nodes["panel"].Parent; got != "right" {
		t.Fatalf("panel parent=%q want=right", got)
	}
}

func TestRelocatingTablePreservesStableSelection(t *testing.T) {
	e := New(protocol.DefaultLimits(), 32, NewNoopActions())
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "left", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{}`)})
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "right", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{}`)})
	tableProps := json.RawMessage(`{"columns":[{"title":"Name","width":8}],"rows":[["A"],["B"]],"row_ids":["row-a","row-b"],"selectable":true}`)
	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "table", Type: protocol.NodeTable, Parent: "left", Props: tableProps})

	if err := e.SetTableSelection("table", 1); err != nil {
		t.Fatal(err)
	}
	before, ok := e.SelectedTableRow("table")
	if !ok || before.Index != 1 || before.RowID != "row-b" {
		t.Fatalf("before selection=%+v ok=%v", before, ok)
	}

	applyRelocationOp(t, e, protocol.Operation{V: 1, Seq: 4, Op: protocol.OpUpsert, ID: "table", Type: protocol.NodeTable, Parent: "right", Props: tableProps})

	after, ok := e.SelectedTableRow("table")
	if !ok {
		t.Fatal("selection disappeared after relocation")
	}
	if after != before {
		t.Fatalf("selection=%+v want=%+v", after, before)
	}
}
