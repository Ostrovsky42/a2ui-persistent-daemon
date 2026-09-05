package engine

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"a2ui/protocol"
	a2runtime "a2ui/runtime"
)

func op(seq int64, id string, typ protocol.NodeType, props string) protocol.Operation {
	return protocol.Operation{V: 1, Seq: seq, Op: protocol.OpUpsert, ID: id, Type: typ, Parent: "root", Props: json.RawMessage(props)}
}

func TestEngineRejectsRuntimeConflictWithoutCommittingDocument(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(1, "a", protocol.NodeActions, `{"items":[{"key":"r","label":"Reload","action":"reload"}]}`)); err != nil {
		t.Fatal(err)
	}
	before := e.Document()
	err := e.Apply(op(2, "b", protocol.NodeActions, `{"items":[{"key":"r","label":"Reboot","action":"reboot"}]}`))
	if err == nil || err.Code != "runtime.binding_conflict" {
		t.Fatalf("err=%#v", err)
	}
	after := e.Document()
	if after.Revision != before.Revision {
		t.Fatalf("revision changed %d -> %d", before.Revision, after.Revision)
	}
	if _, ok := after.Nodes["b"]; ok {
		t.Fatal("conflicting candidate committed")
	}
}

func TestCommitAcknowledgesOnlyAfterRendererPublication(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(1, "x", protocol.NodeText, `{"text":"x"}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpCommit, Frame: "boot"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.NextEvent(); ok {
		t.Fatal("commit was acknowledged before renderer publication")
	}
	if !e.NeedsPublish() {
		t.Fatal("pending commit must request publication")
	}
	if err := e.Publish(); err != nil {
		t.Fatal(err)
	}
	ev, ok := e.NextEvent()
	if !ok || ev.Ev != "committed" || ev.Revision != 1 || ev.ThroughSeq != 2 || ev.Frame != "boot" {
		t.Fatalf("ev=%+v ok=%v", ev, ok)
	}
	if e.NeedsPublish() {
		t.Fatal("successful publication left runtime dirty")
	}
}

func TestInputSubmitUsesLocalValueAndRegisteredAction(t *testing.T) {
	reg := NewNoopActions()
	_ = reg.Register("cmd.submit", func(context.Context, json.RawMessage) error { return nil })
	e := New(protocol.DefaultLimits(), 8, reg)
	if err := e.Apply(op(1, "in", protocol.NodeInput, `{"value":"agent","action":"cmd.submit"}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.SetInput("in", "user typed"); err != nil {
		t.Fatal(err)
	}
	if err := e.Submit("in"); err != nil {
		t.Fatal(err)
	}
	ev, ok := e.NextEvent()
	if !ok || ev.Ev != "submit" || ev.Value != "user typed" || ev.Action != "cmd.submit" {
		t.Fatalf("ev=%+v", ev)
	}
}

func TestHandleKeyDispatchesDeterministicAction(t *testing.T) {
	reg := NewNoopActions()
	called := false
	_ = reg.Register("reload", func(context.Context, json.RawMessage) error { called = true; return nil })
	e := New(protocol.DefaultLimits(), 8, reg)
	if err := e.Apply(op(1, "a", protocol.NodeActions, `{"items":[{"key":"r","label":"Reload","action":"reload"}]}`)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.HandleKey(ctx, "r"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("handler not called")
	}
	ev, ok := e.NextEvent()
	if !ok || ev.Ev != "action_result" || ev.Action != "reload" {
		t.Fatalf("ev=%+v", ev)
	}
}

func TestErrorEventUsesSeparateRelatedMutationSequence(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	err := e.Apply(protocol.Operation{V: 1, Seq: 42, Op: protocol.OpProps, ID: "missing", Props: json.RawMessage(`{"text":"x"}`)})
	if err == nil {
		t.Fatal("expected error")
	}
	ev, ok := e.NextEvent()
	if !ok {
		t.Fatal("missing error event")
	}
	if ev.Ev != "error" || ev.RelatedSeq != 42 || ev.ThroughSeq != 0 {
		t.Fatalf("ev=%+v", ev)
	}
	if ev.Seq == 42 {
		t.Fatal("outbound event seq must be independent from inbound mutation seq")
	}
}

func TestFocusChangeRequiresPublicationWithoutChangingDocumentRevision(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(1, "a", protocol.NodeInput, `{}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(op(2, "b", protocol.NodeInput, `{}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(); err != nil {
		t.Fatal(err)
	}
	before := e.Document().Revision
	if e.NeedsPublish() {
		t.Fatal("clean engine still needs publication")
	}
	if err := e.Apply(protocol.Operation{V: 1, Seq: 3, Op: protocol.OpFocus, ID: "b"}); err != nil {
		t.Fatal(err)
	}
	if got := e.Document().Revision; got != before {
		t.Fatalf("focus changed document revision: %d -> %d", before, got)
	}
	if !e.NeedsPublish() {
		t.Fatal("focus projection change did not request publication")
	}
	if err := e.Publish(); err != nil {
		t.Fatal(err)
	}
	if e.NeedsPublish() {
		t.Fatal("published focus change remained dirty")
	}
}

func TestSetInputLocalValueDoesNotRequireProtocolPublication(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(1, "in", protocol.NodeInput, `{"value":"agent"}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(); err != nil {
		t.Fatal(err)
	}
	if err := e.SetInput("in", "user"); err != nil {
		t.Fatal(err)
	}
	if e.NeedsPublish() {
		t.Fatal("local input editing must redraw locally without protocol publication")
	}
}

func TestLocalFocusDoesNotRequireProtocolPublication(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(1, "a", protocol.NodeInput, `{}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(op(2, "b", protocol.NodeInput, `{}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(); err != nil {
		t.Fatal(err)
	}
	before := e.Document().Revision
	if err := e.Focus("b"); err != nil {
		t.Fatal(err)
	}
	if got := e.FocusedID(); got != "b" {
		t.Fatalf("focus=%q", got)
	}
	if got := e.Document().Revision; got != before {
		t.Fatalf("local focus changed document revision: %d -> %d", before, got)
	}
	if e.NeedsPublish() {
		t.Fatal("local focus navigation must redraw without protocol publication")
	}
}

func TestCommitAdmissionCannotCreateUnpublishableBatch(t *testing.T) {
	e := New(protocol.DefaultLimits(), 1, NewNoopActions())
	if err := e.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpCommit, Frame: "first"}); err != nil {
		t.Fatal(err)
	}
	err := e.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpCommit, Frame: "second"})
	if err == nil || err.Code != "runtime.backpressure_exceeded" {
		t.Fatalf("second commit must be rejected before creating an impossible batch: %#v", err)
	}
	// The rejection itself is a critical error event. Once the consumer drains it,
	// the already-accepted first barrier must still be publishable.
	if ev, ok := e.NextEvent(); !ok || ev.Ev != "error" {
		t.Fatalf("missing rejection event: %+v %v", ev, ok)
	}
	if err := e.Publish(); err != nil {
		t.Fatalf("accepted barrier became permanently unpublishable: %v", err)
	}
	if ev, ok := e.NextEvent(); !ok || ev.Ev != "committed" || ev.ThroughSeq != 1 {
		t.Fatalf("commit event=%+v ok=%v", ev, ok)
	}
}

func TestTableSelectionEngineAPIIsLocalAndActivatesStableRow(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(50, "jobs", protocol.NodeTable, `{"selectable":true,"action":"open_job","columns":[{"title":"Job","width":10}],"rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Publish(); err != nil {
		t.Fatal(err)
	}
	beforeRevision := e.Document().Revision
	if err := e.MoveTableSelection("jobs", 1); err != nil {
		t.Fatal(err)
	}
	sel, ok := e.SelectedTableRow("jobs")
	if !ok || sel.Index != 1 || sel.RowID != "job-b" {
		t.Fatalf("selection=%+v ok=%v", sel, ok)
	}
	if e.Document().Revision != beforeRevision {
		t.Fatal("local table selection mutated Document")
	}
	if e.NeedsPublish() {
		t.Fatal("local table selection required protocol publication")
	}
	if err := e.ActivateTableSelection("jobs"); err != nil {
		t.Fatal(err)
	}
	ev, ok := e.NextEvent()
	if !ok {
		t.Fatal("missing select event")
	}
	if ev.Ev != "select" || ev.ID != "jobs" || ev.Row != 1 || ev.RowID != "job-b" || ev.Action != "open_job" {
		t.Fatalf("select event=%+v", ev)
	}
}

func TestActivateEmptySelectableTableEmitsNothing(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(60, "jobs", protocol.NodeTable, `{"selectable":true,"action":"open_job","rows":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.ActivateTableSelection("jobs"); err != nil {
		t.Fatal(err)
	}
	if ev, ok := e.NextEvent(); ok {
		t.Fatalf("empty table emitted event: %+v", ev)
	}
}

func TestPresentationSnapshotIsAtomicCopy(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(70, "in", protocol.NodeInput, `{"value":"agent"}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(op(71, "jobs", protocol.NodeTable, `{"selectable":true,"rows":[["A"]],"row_ids":["job-a"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := e.SetInput("in", "local"); err != nil {
		t.Fatal(err)
	}
	snap := e.PresentationSnapshot()
	if snap.InputValues["in"] != "local" {
		t.Fatalf("snapshot input=%q", snap.InputValues["in"])
	}
	if snap.TableSelections["jobs"].RowID != "job-a" {
		t.Fatalf("snapshot selection=%+v", snap.TableSelections["jobs"])
	}
	if snap.Bindings == nil {
		t.Fatal("snapshot bindings map is nil")
	}

	snap.InputValues["in"] = "corrupt"
	snap.TableSelections["jobs"] = a2runtime.TableSelection{Index: 99, RowID: "corrupt"}
	snap.Bindings["x"] = a2runtime.Binding{NodeID: "corrupt"}
	delete(snap.Document.Nodes, "in")

	if got := e.InputValue("in"); got != "local" {
		t.Fatalf("snapshot map aliases engine: %q", got)
	}
	if sel, ok := e.SelectedTableRow("jobs"); !ok || sel.RowID != "job-a" {
		t.Fatalf("snapshot selection aliases engine: %+v ok=%v", sel, ok)
	}
	if _, ok := e.Document().Nodes["in"]; !ok {
		t.Fatal("snapshot document aliases engine")
	}
	if e.HasKeyBinding("x") {
		t.Fatal("snapshot bindings alias engine")
	}
}

func TestTableSelectIsCriticalAndReportsBackpressure(t *testing.T) {
	limits := protocol.DefaultLimits()
	eng := New(limits, 1, NewNoopActions())
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "tbl", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"action":"open","columns":[{"title":"ID"}],"rows":[["a"]],"row_ids":["row-a"]}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.ActivateTableSelection("tbl"); perr != nil {
		t.Fatal(perr)
	}
	perr := eng.ActivateTableSelection("tbl")
	if perr == nil || perr.Code != "runtime.backpressure_exceeded" {
		t.Fatalf("expected explicit critical-event backpressure, got %+v", perr)
	}
	ev, ok := eng.NextEvent()
	if !ok || ev.Ev != "select" || ev.RowID != "row-a" {
		t.Fatalf("expected first select preserved in broker, got %+v ok=%v", ev, ok)
	}
}

func TestPublicationGenerationRejectsStaleAckAndPublishesExactCurrentFrame(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	if err := e.Apply(op(80, "x", protocol.NodeText, `{"text":"one"}`)); err != nil {
		t.Fatal(err)
	}
	first := e.PresentationSnapshot()
	if !first.PublicationPending || first.PublicationGeneration == 0 {
		t.Fatalf("first snapshot publication=%+v", first)
	}
	if err := e.Apply(protocol.Operation{V: 1, Seq: 81, Op: protocol.OpProps, ID: "x", Props: json.RawMessage(`{"text":"two"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(protocol.Operation{V: 1, Seq: 82, Op: protocol.OpCommit, Frame: "latest"}); err != nil {
		t.Fatal(err)
	}
	latest := e.PresentationSnapshot()
	if latest.PublicationGeneration <= first.PublicationGeneration {
		t.Fatalf("publication generation did not advance: first=%d latest=%d", first.PublicationGeneration, latest.PublicationGeneration)
	}

	published, err := e.PublishGeneration(first.PublicationGeneration)
	if err != nil {
		t.Fatalf("stale ack returned engine error: %v", err)
	}
	if published {
		t.Fatal("stale publication ack published newer frame")
	}
	if !e.NeedsPublish() {
		t.Fatal("stale ack cleared pending publication")
	}
	if _, ok := e.NextEvent(); ok {
		t.Fatal("stale ack emitted committed")
	}

	published, err = e.PublishGeneration(latest.PublicationGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if !published {
		t.Fatal("current generation was not published")
	}
	if e.NeedsPublish() {
		t.Fatal("current publication remained pending")
	}
	ev, ok := e.NextEvent()
	if !ok || ev.Ev != "committed" || ev.Frame != "latest" || ev.ThroughSeq != 82 {
		t.Fatalf("committed=%+v ok=%v", ev, ok)
	}

	published, err = e.PublishGeneration(latest.PublicationGeneration)
	if err != nil {
		t.Fatal(err)
	}
	if published {
		t.Fatal("duplicate ack republished generation")
	}
	if ev, ok := e.NextEvent(); ok {
		t.Fatalf("duplicate ack emitted event %+v", ev)
	}
}

func TestPresentationSnapshotCarriesPublicationMetadataAtomically(t *testing.T) {
	e := New(protocol.DefaultLimits(), 8, NewNoopActions())
	clean := e.PresentationSnapshot()
	if clean.PublicationPending || clean.PublicationGeneration != 0 {
		t.Fatalf("new engine publication metadata=%+v", clean)
	}
	if err := e.Apply(op(90, "x", protocol.NodeText, `{"text":"x"}`)); err != nil {
		t.Fatal(err)
	}
	dirty := e.PresentationSnapshot()
	if !dirty.PublicationPending || dirty.PublicationGeneration == 0 {
		t.Fatalf("dirty snapshot publication metadata=%+v", dirty)
	}
	if dirty.Document.Revision != 1 {
		t.Fatalf("snapshot revision=%d", dirty.Document.Revision)
	}
}
