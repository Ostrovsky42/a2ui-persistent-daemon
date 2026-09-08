package document

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func up(seq int64, id string, typ protocol.NodeType, parent, props string) protocol.Operation {
	return protocol.Operation{V: 1, Seq: seq, Op: protocol.OpUpsert, ID: id, Type: typ, Parent: parent, Props: json.RawMessage(props)}
}

func applyOK(t *testing.T, d Document, op protocol.Operation) (Document, Effect) {
	t.Helper()
	next, eff, err := Apply(d, op, protocol.DefaultLimits())
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	return next, eff
}

func TestPropsWorksForBoxBecauseDocumentOwnsNodes(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "main", protocol.NodeBox, "root", `{"gap":0}`))
	next, _, err := Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "main", Props: json.RawMessage(`{"gap":2}`)}, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	var gap int
	_ = json.Unmarshal(next.Nodes["main"].Props["gap"], &gap)
	if gap != 2 {
		t.Fatalf("gap=%d", gap)
	}
}

func TestFailedMutationRollsBackAuthoritativeState(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "x", protocol.NodeText, "root", `{"text":"good"}`))
	before := d.Clone()
	_, _, err := Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "x", Props: json.RawMessage(`{"style":{"fg":"#fff"}}`)}, protocol.DefaultLimits())
	if err == nil {
		t.Fatal("expected invalid color")
	}
	if !reflect.DeepEqual(d, before) {
		t.Fatal("failed mutation changed original document")
	}
}

func TestOnlyBoxCanBeParent(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "leaf", protocol.NodeText, "root", `{}`))
	_, _, err := Apply(d, up(2, "child", protocol.NodeText, "leaf", `{}`), protocol.DefaultLimits())
	if err == nil || err.Code != "document.parent_not_container" {
		t.Fatalf("got %#v", err)
	}
}

func TestUpsertIsFullReplaceAndPropsIsShallowMerge(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "t", protocol.NodeTable, "root", `{"columns":[{"title":"A","width":3}],"rows":[["x"]],"selectable":true}`))
	d, _, err := Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "t", Props: json.RawMessage(`{"rows":[["y"]]}`)}, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	var cols []any
	_ = json.Unmarshal(d.Nodes["t"].Props["columns"], &cols)
	if len(cols) != 1 {
		t.Fatalf("merge lost columns: %s", d.Nodes["t"].Props["columns"])
	}
	d, _, err = Apply(d, up(3, "t", protocol.NodeTable, "", `{"rows":[]}`), protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(d.Nodes["t"].Props["columns"], &cols)
	if len(cols) != 0 {
		t.Fatalf("replace retained columns: %s", d.Nodes["t"].Props["columns"])
	}
}

func TestInvalidCreateDoesNotPoisonLaterRecovery(t *testing.T) {
	d := New()
	next, _, err := Apply(d, up(1, "x", protocol.NodeText, "root", `{"style":{"fg":"#bad"}}`), protocol.DefaultLimits())
	if err == nil {
		t.Fatal("expected failure")
	}
	if _, ok := next.Nodes["x"]; ok {
		t.Fatal("invalid node must not be committed")
	}
	next, _, err = Apply(d, up(2, "x", protocol.NodeText, "root", `{"text":"recovered"}`), protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.Nodes["x"]; !ok {
		t.Fatal("valid recovery did not create node")
	}
}

func TestRemoveFocusedConcernIsReportedAsEffectAndRootImmutable(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "box", protocol.NodeBox, "root", `{}`))
	d, _ = applyOK(t, d, up(2, "x", protocol.NodeInput, "box", `{}`))
	next, eff, err := Apply(d, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpRemove, ID: "box"}, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if len(eff.Removed) != 2 {
		t.Fatalf("removed=%v", eff.Removed)
	}
	if _, ok := next.Nodes["x"]; ok {
		t.Fatal("descendant survived")
	}
	_, _, err = Apply(next, protocol.Operation{V: 1, Seq: 4, Op: protocol.OpRemove, ID: "root"}, protocol.DefaultLimits())
	if err == nil || err.Code != "document.root_immutable" {
		t.Fatalf("got %#v", err)
	}
}

func TestNodeAndDepthLimitsAreTransactional(t *testing.T) {
	l := protocol.DefaultLimits()
	l.MaxNodes = 2
	l.MaxDepth = 1
	d := New()
	d, _, err := Apply(d, up(1, "a", protocol.NodeBox, "root", `{}`), l)
	if err != nil {
		t.Fatal(err)
	}
	before := d.Clone()
	_, _, err = Apply(d, up(2, "b", protocol.NodeText, "a", `{}`), l)
	if err == nil {
		t.Fatal("expected limit error")
	}
	if !reflect.DeepEqual(d, before) {
		t.Fatal("limit failure mutated doc")
	}
}

func TestTextRetentionIsBounded(t *testing.T) {
	l := protocol.DefaultLimits()
	l.MaxTextBytesPerNode = 5
	l.MaxTotalTextBytes = 5
	d := New()
	d, _, err := Apply(d, up(1, "log", protocol.NodeViewport, "root", `{}`), l)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err = Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpText, ID: "log", Text: "12345"}, l)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = Apply(d, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpText, ID: "log", Text: "6"}, l)
	if err == nil || err.Code != "resource.text_too_large" {
		t.Fatalf("got %#v", err)
	}
}

func TestTypeChangeRecreatesRuntimeStateEffect(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "x", protocol.NodeInput, "root", `{"value":"a"}`))
	next, eff, err := Apply(d, up(2, "x", protocol.NodeText, "", `{"text":"b"}`), protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !eff.TypeChanged || next.Nodes["x"].Type != protocol.NodeText {
		t.Fatalf("effect=%+v node=%+v", eff, next.Nodes["x"])
	}
}

func TestValidateDetectsBrokenDocument(t *testing.T) {
	d := New()
	d.Nodes["orphan"] = Node{ID: "orphan", Type: protocol.NodeText, Parent: "missing", Props: map[string]json.RawMessage{}}
	if err := Validate(d, protocol.DefaultLimits()); err == nil {
		t.Fatal("broken document accepted")
	}
}

func TestNegotiatedTableLimitsApplyToPropsAndMayExceedDefaults(t *testing.T) {
	l := protocol.DefaultLimits()
	l.MaxTableColumns = 100
	l.MaxTableRows = 2
	cols := make([]map[string]any, 65)
	for i := range cols {
		cols[i] = map[string]any{"title": "C", "width": 1}
	}
	b, _ := json.Marshal(map[string]any{"columns": cols})
	d := New()
	d, _, err := Apply(d, protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "t", Type: protocol.NodeTable, Parent: "root", Props: b}, l)
	if err != nil {
		t.Fatalf("custom limit should permit 65 columns: %v", err)
	}
	rows, _ := json.Marshal(map[string]any{"rows": [][]string{{"1"}, {"2"}, {"3"}}})
	_, _, err = Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "t", Props: rows}, l)
	if err == nil || err.Code != "resource.table_rows_limit" {
		t.Fatalf("got %#v", err)
	}
}

func TestTypeChangeRemovesRetainedTextAccounting(t *testing.T) {
	l := protocol.DefaultLimits()
	d := New()
	d, _, _ = Apply(d, up(1, "x", protocol.NodeViewport, "root", `{}`), l)
	d, _, _ = Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpText, ID: "x", Text: "abc"}, l)
	d, _, err := Apply(d, up(3, "x", protocol.NodeInput, "", `{}`), l)
	if err != nil {
		t.Fatal(err)
	}
	if d.TotalTextBytes != 0 {
		t.Fatalf("stale total text bytes=%d", d.TotalTextBytes)
	}
	if inv := Validate(d, l); inv != nil {
		t.Fatal(inv)
	}
}

func TestRootCannotBeUpserted(t *testing.T) {
	d := New()
	before := d.Clone()
	_, _, err := Apply(d, up(1, "root", protocol.NodeBox, "", `{"gap":3}`), protocol.DefaultLimits())
	if err == nil || err.Code != "document.root_immutable" {
		t.Fatalf("got %#v", err)
	}
	if !reflect.DeepEqual(d, before) {
		t.Fatal("root upsert mutated document")
	}
}

func TestNoOpMutationsDoNotAdvanceDocumentRevision(t *testing.T) {
	limits := protocol.DefaultLimits()
	d, _ := applyOK(t, New(), up(1, "x", protocol.NodeText, "root", `{"text":"same"}`))
	baseRevision := d.Revision

	next, eff, err := Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "x", Props: json.RawMessage(`{}`)}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Changed || next.Revision != baseRevision {
		t.Fatalf("empty props changed document: eff=%+v revision=%d want=%d", eff, next.Revision, baseRevision)
	}

	next, eff, err = Apply(d, protocol.Operation{V: 1, Seq: 3, Op: protocol.OpText, ID: "x", Text: ""}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Changed || next.Revision != baseRevision {
		t.Fatalf("empty text changed document: eff=%+v revision=%d want=%d", eff, next.Revision, baseRevision)
	}

	next, eff, err = Apply(d, up(4, "x", protocol.NodeText, "", `{"text":"same"}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	if eff.Changed || next.Revision != baseRevision {
		t.Fatalf("identical upsert changed document: eff=%+v revision=%d want=%d", eff, next.Revision, baseRevision)
	}
}

func TestAggregateDocumentByteLimitIsTransactional(t *testing.T) {
	limits := protocol.DefaultLimits()
	limits.MaxDocumentBytes = 512
	d := New()
	before := d.Clone()
	large := string(make([]byte, 600))
	props, _ := json.Marshal(map[string]any{"text": large})
	next, _, err := Apply(d, protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "large", Type: protocol.NodeText, Parent: "root", Props: props}, limits)
	if err == nil || err.Code != "resource.document_bytes_limit" {
		t.Fatalf("got %#v", err)
	}
	if !reflect.DeepEqual(next, before) || !reflect.DeepEqual(d, before) {
		t.Fatal("document-byte limit failure mutated authoritative state")
	}
}

func TestViewportCanBeParentButLeavesCannot(t *testing.T) {
	limits := protocol.DefaultLimits()
	d := New()
	var err *protocol.Error
	d, _, err = Apply(d, up(100, "vp", protocol.NodeViewport, "root", `{"height":4}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err = Apply(d, up(101, "line", protocol.NodeText, "vp", `{"text":"hello"}`), limits)
	if err != nil {
		t.Fatalf("viewport should accept children: %v", err)
	}
	if got := d.Nodes["vp"].Children; !reflect.DeepEqual(got, []string{"line"}) {
		t.Fatalf("viewport children=%v", got)
	}

	leaves := []protocol.NodeType{
		protocol.NodeText,
		protocol.NodeTable,
		protocol.NodeInput,
		protocol.NodeActions,
		protocol.NodeProgress,
	}
	for i, typ := range leaves {
		id := fmt.Sprintf("leaf-%d", i)
		var next Document
		next, _, err = Apply(d, up(int64(110+i), id, typ, "root", `{}`), limits)
		if err != nil {
			t.Fatalf("create %s: %v", typ, err)
		}
		_, _, err = Apply(next, up(int64(120+i), id+"-child", protocol.NodeText, id, `{}`), limits)
		if err == nil || err.Code != "document.parent_not_container" {
			t.Fatalf("%s unexpectedly accepted child: %#v", typ, err)
		}
	}
}

func TestViewportSubtreeRemovalAndTypeChangeInvariant(t *testing.T) {
	limits := protocol.DefaultLimits()
	d := New()
	var err *protocol.Error
	d, _, err = Apply(d, up(200, "vp", protocol.NodeViewport, "root", `{}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err = Apply(d, up(201, "line", protocol.NodeText, "vp", `{"text":"x"}`), limits)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = Apply(d, up(202, "vp", protocol.NodeText, "", `{}`), limits)
	if err == nil || err.Code != "document.parent_not_container" {
		t.Fatalf("viewport with children changed to leaf: %#v", err)
	}

	next, eff, err := Apply(d, protocol.Operation{V: 1, Seq: 203, Op: protocol.OpRemove, ID: "vp"}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := next.Nodes["vp"]; ok {
		t.Fatal("viewport survived subtree removal")
	}
	if _, ok := next.Nodes["line"]; ok {
		t.Fatal("viewport child survived subtree removal")
	}
	if !reflect.DeepEqual(eff.Removed, []string{"line", "vp"}) && !reflect.DeepEqual(eff.Removed, []string{"vp", "line"}) {
		t.Fatalf("removed=%v", eff.Removed)
	}
}

func TestExplicitPresentationPropsAreRetainedAsSemanticMetadata(t *testing.T) {
	d, _ := applyOK(t, New(), up(1, "card", protocol.NodeBox, "root", `{"variant":"card"}`))
	if d.Nodes["card"].ExplicitProps["border"] {
		t.Fatal("default border must not be marked explicit")
	}
	before := d.Revision
	next, eff, err := Apply(d, protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "card", Props: json.RawMessage(`{"border":"none"}`)}, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !eff.Changed || next.Revision != before+1 {
		t.Fatalf("explicit default changes presentation semantics: effect=%+v revision=%d want=%d", eff, next.Revision, before+1)
	}
	if !next.Nodes["card"].ExplicitProps["border"] {
		t.Fatal("explicit border marker was not retained")
	}

	replaced, eff, err := Apply(next, up(3, "card", protocol.NodeBox, "", `{"variant":"card"}`), protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if !eff.Changed {
		t.Fatal("full upsert that removes explicit override must be a semantic change")
	}
	if replaced.Nodes["card"].ExplicitProps["border"] {
		t.Fatal("full upsert must reset omitted explicit markers")
	}
}
