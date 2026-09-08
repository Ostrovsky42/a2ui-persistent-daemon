package bubbletea

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestP03EnterSelectCarriesVisibleCausalityAndNewCursor(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng,
		p03WorkersTable(1, "workers"),
		protocol.Operation{V: protocol.Version, Seq: 2, Op: protocol.OpCommit, Frame: "workers-visible"},
	)
	if perr := eng.Focus("workers"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)

	// View is the real Bubble Tea visibility boundary: only after this frame is
	// rendered may the adapter acknowledge its publication generation.
	_ = model.View()
	committed, publicationCursor, ok := eng.NextEventWithCursor()
	if !ok || committed.Ev != "committed" || committed.Frame != "workers-visible" {
		t.Fatalf("publication boundary event=%+v cursor=%d ok=%v", committed, publicationCursor, ok)
	}
	visible := eng.PresentationSnapshot()
	if visible.PublicationPending {
		t.Fatal("visible frame still reports pending publication")
	}

	model = pressP03(t, model, tea.KeyDown)
	assertP03NoEvents(t, eng)
	model = pressP03(t, model, tea.KeyEnter)

	selected, selectCursor, ok := eng.NextEventWithCursor()
	if !ok {
		t.Fatal("Enter emitted no select event")
	}
	if selected.Ev != "select" || selected.RowID != "worker-b" || selected.Row != 1 {
		t.Fatalf("unexpected select event: %+v", selected)
	}
	if selected.Frame != "workers-visible" {
		t.Fatalf("select frame=%q, want currently visible frame", selected.Frame)
	}
	if selected.Revision != visible.Document.Revision {
		t.Fatalf("select revision=%d, visible document revision=%d", selected.Revision, visible.Document.Revision)
	}
	if selectCursor <= publicationCursor {
		t.Fatalf("select cursor=%d must be newer than publication boundary=%d", selectCursor, publicationCursor)
	}
	assertP03NoEvents(t, eng)
}

func TestP03FrozenSchemaRejectsNamedNavigationActionKey(t *testing.T) {
	eng := newTestEngine()
	perr := eng.Apply(protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "actions",
		Type:   protocol.NodeActions,
		Parent: "root",
		Props:  json.RawMessage(`{"items":[{"key":"down","label":"Shadow","action":"shadow"}]}`),
	})
	if perr == nil {
		t.Fatal("named special-key action unexpectedly passed frozen V1 schema")
	}
	if perr.Code != "schema.invalid_props" {
		t.Fatalf("named special-key action error=%v, want schema.invalid_props", perr)
	}
}

func TestP03FocusedInputOwnsPrintableRuneBeforeSemanticAction(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng,
		protocol.Operation{V: protocol.Version, Seq: 1, Op: protocol.OpUpsert, ID: "query", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"","action":"search"}`)},
		protocol.Operation{V: protocol.Version, Seq: 2, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Retry","action":"retry"}]}`)},
	)
	if perr := eng.Focus("query"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	model = updated.(Model)
	if got := eng.InputValue("query"); got != "r" {
		t.Fatalf("focused input value=%q, want local rune insertion", got)
	}
	assertP03NoEvents(t, eng)
}
