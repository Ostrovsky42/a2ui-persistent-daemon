package runtime

import (
	"encoding/json"
	"reflect"
	"testing"

	"a2ui/document"
	"a2ui/protocol"
)

func apply(t *testing.T, s *State, d document.Document, op protocol.Operation) document.Document {
	t.Helper()
	next, eff, err := document.Apply(d, op, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(d, next, op, eff); err != nil {
		t.Fatal(err)
	}
	return next
}
func up(seq int64, id string, typ protocol.NodeType, parent, props string) protocol.Operation {
	return protocol.Operation{V: 1, Seq: seq, Op: protocol.OpUpsert, ID: id, Type: typ, Parent: parent, Props: json.RawMessage(props)}
}

func TestFocusReconcilesAfterRemovalAndNonFocusableChange(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(1, "a", protocol.NodeInput, "root", `{}`))
	d = apply(t, s, d, up(2, "b", protocol.NodeInput, "root", `{}`))
	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpFocus, ID: "b"})
	if s.FocusedID != "b" {
		t.Fatalf("focus=%q", s.FocusedID)
	}
	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 4, Op: protocol.OpRemove, ID: "b"})
	if s.FocusedID != "a" {
		t.Fatalf("expected deterministic fallback a, got %q", s.FocusedID)
	}
	d = apply(t, s, d, up(5, "t", protocol.NodeTable, "root", `{"selectable":true}`))
	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 6, Op: protocol.OpFocus, ID: "t"})
	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 7, Op: protocol.OpProps, ID: "t", Props: json.RawMessage(`{"selectable":false}`)})
	if s.FocusedID != "a" {
		t.Fatalf("non-focusable stale focus: %q", s.FocusedID)
	}
}

func TestInputForceIsOneShot(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(1, "in", protocol.NodeInput, "root", `{"value":"agent"}`))
	if s.InputValues["in"] != "agent" {
		t.Fatal(s.InputValues)
	}
	s.InputValues["in"] = "user typed"
	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "in", Props: json.RawMessage(`{"placeholder":"new"}`)})
	if s.InputValues["in"] != "user typed" {
		t.Fatalf("unrelated props overwrote input: %q", s.InputValues["in"])
	}
	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpProps, ID: "in", Props: json.RawMessage(`{"value":"","force":true}`)})
	if s.InputValues["in"] != "" {
		t.Fatalf("force did not apply: %q", s.InputValues["in"])
	}
	s.InputValues["in"] = "again"
	_ = apply(t, s, d, protocol.Operation{V: 1, Seq: 4, Op: protocol.OpProps, ID: "in", Props: json.RawMessage(`{"placeholder":"x"}`)})
	if s.InputValues["in"] != "again" {
		t.Fatalf("force stuck: %q", s.InputValues["in"])
	}
}

func TestDuplicateActionBindingRejectedDeterministically(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(1, "a", protocol.NodeActions, "root", `{"items":[{"key":"r","label":"Reload","action":"reload"}]}`))
	next, eff, err := document.Apply(d, up(2, "b", protocol.NodeActions, "root", `{"items":[{"key":"r","label":"Reboot","action":"reboot"}]}`), protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(d, next, up(2, "b", protocol.NodeActions, "root", `{"items":[{"key":"r","label":"Reboot","action":"reboot"}]}`), eff); err == nil || err.Code != "runtime.binding_conflict" {
		t.Fatalf("expected binding conflict, got %#v", err)
	}
}

func TestCommitDoesNotPretendRendererPublished(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(1, "x", protocol.NodeText, "root", `{"text":"x"}`))
	if s.PublishedRevision == d.Revision {
		t.Fatal("mutation should be dirty before publish")
	}
	op := protocol.Operation{V: 1, Seq: 2, Op: protocol.OpCommit, Frame: "boot"}
	next, eff, err := document.Apply(d, op, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(d, next, op, eff); err != nil {
		t.Fatal(err)
	}
	if s.PublishedRevision == d.Revision {
		t.Fatal("commit alone marked a frame as published")
	}
	s.Publish()
	if s.PublishedRevision != d.Revision {
		t.Fatalf("published=%d want=%d", s.PublishedRevision, d.Revision)
	}
}

func TestCreatingFocusableNodeDoesNotAutoFocus(t *testing.T) {
	s := NewState()
	d := document.New()
	_ = apply(t, s, d, up(1, "in", protocol.NodeInput, "root", `{}`))
	if s.FocusedID != "" {
		t.Fatalf("creation stole focus: %q", s.FocusedID)
	}
}

func TestTableSelectionLivesInRuntimeAndFollowsStableRowID(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(20, "jobs", protocol.NodeTable, "root", `{"selectable":true,"rows":[["A"],["B"],["C"]],"row_ids":["job-a","job-b","job-c"]}`))

	sel, ok := s.TableSelection("jobs")
	if !ok || sel.Index != 0 || sel.RowID != "job-a" {
		t.Fatalf("initial selection=%+v ok=%v", sel, ok)
	}
	if err := s.SetTableSelection(d, "jobs", 1); err != nil {
		t.Fatal(err)
	}
	sel, _ = s.TableSelection("jobs")
	if sel.Index != 1 || sel.RowID != "job-b" {
		t.Fatalf("selected=%+v", sel)
	}
	publicationBefore := s.PublicationGeneration

	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 21, Op: protocol.OpProps, ID: "jobs", Props: json.RawMessage(`{"rows":[["C"],["A"],["B"]],"row_ids":["job-c","job-a","job-b"]}`)})
	sel, ok = s.TableSelection("jobs")
	if !ok || sel.Index != 2 || sel.RowID != "job-b" {
		t.Fatalf("selection did not follow row ID: %+v ok=%v", sel, ok)
	}
	if s.PublicationGeneration <= publicationBefore {
		t.Fatal("agent table mutation must still require publication")
	}
}

func TestTableSelectionReconcilesRemovalEmptyAndSelectableState(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(30, "jobs", protocol.NodeTable, "root", `{"selectable":true,"rows":[["A"],["B"],["C"]],"row_ids":["a","b","c"]}`))
	if err := s.SetTableSelection(d, "jobs", 2); err != nil {
		t.Fatal(err)
	}

	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 31, Op: protocol.OpProps, ID: "jobs", Props: json.RawMessage(`{"rows":[["A"],["B"]],"row_ids":["a","b"]}`)})
	if sel, ok := s.TableSelection("jobs"); !ok || sel.Index != 1 || sel.RowID != "b" {
		t.Fatalf("removed selected row did not clamp: %+v ok=%v", sel, ok)
	}

	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 32, Op: protocol.OpProps, ID: "jobs", Props: json.RawMessage(`{"rows":[],"row_ids":[]}`)})
	if _, ok := s.TableSelection("jobs"); ok {
		t.Fatal("empty table must have no selection")
	}

	d = apply(t, s, d, protocol.Operation{V: 1, Seq: 33, Op: protocol.OpProps, ID: "jobs", Props: json.RawMessage(`{"rows":[["A"]],"row_ids":["a"],"selectable":false}`)})
	if _, ok := s.TableSelection("jobs"); ok {
		t.Fatal("non-selectable table retained selection")
	}
}

func TestLocalTableSelectionChangesRenderOnlyNotPublication(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(40, "jobs", protocol.NodeTable, "root", `{"selectable":true,"rows":[["A"],["B"]]}`))
	s.Publish()
	beforeRender := s.RenderGeneration
	beforePublication := s.PublicationGeneration
	if err := s.MoveTableSelection(d, "jobs", 1); err != nil {
		t.Fatal(err)
	}
	if sel, ok := s.TableSelection("jobs"); !ok || sel.Index != 1 || sel.RowID != "" {
		t.Fatalf("selection=%+v ok=%v", sel, ok)
	}
	if s.RenderGeneration <= beforeRender {
		t.Fatal("local selection did not request redraw")
	}
	if s.PublicationGeneration != beforePublication || s.NeedsPublish() {
		t.Fatal("local selection must not require protocol publication")
	}
}

func TestScrollableViewportParticipatesInFocusOrder(t *testing.T) {
	s := NewState()
	d := document.New()
	d = apply(t, s, d, up(50, "input", protocol.NodeInput, "root", `{}`))
	d = apply(t, s, d, up(51, "passive", protocol.NodeViewport, "root", `{"scrollable":false}`))
	d = apply(t, s, d, up(52, "logs", protocol.NodeViewport, "root", `{"scrollable":true}`))
	d = apply(t, s, d, up(53, "table", protocol.NodeTable, "root", `{"selectable":true,"rows":[["x"]]}`))
	got := FocusableIDs(d)
	want := []string{"input", "logs", "table"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("focus order=%v want=%v", got, want)
	}
}
