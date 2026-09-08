package bubbletea

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"a2ui/engine"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUXAcknowledgesSelectLocallyUntilNextPublication(t *testing.T) {
	eng := newTestEngine()
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "services", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"action":"service.choose","rows":[["production"],["staging"]],"row_ids":["production","staging"]}`)},
		{V: 1, Seq: 2, Op: protocol.OpFocus, ID: "services"},
		{V: 1, Seq: 3, Op: protocol.OpCommit, Frame: "services"},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Publish(); err != nil {
		t.Fatal(err)
	}
	if _, ok := eng.NextEvent(); !ok {
		t.Fatal("missing initial committed event")
	}

	before := eng.PresentationSnapshot()
	model := NewModel(eng, DefaultTheme, nil)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	ev, ok := eng.NextEvent()
	if !ok || ev.Ev != "select" || ev.RowID != "production" {
		t.Fatalf("select=%+v ok=%v", ev, ok)
	}
	if _, ok := eng.NextEvent(); ok {
		t.Fatal("select acknowledgement emitted a second semantic event")
	}
	frame := model.View()
	if !strings.Contains(frame, "Selected: production") || !strings.Contains(frame, "⠋ Waiting for agent") || strings.Contains(frame, "Enter Select") {
		t.Fatalf("select has no local acknowledgement before a new publication:\n%s", frame)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if frame := model.View(); !strings.Contains(frame, "Selected: production") {
		t.Fatalf("local navigation lost committed choice:\n%s", frame)
	}
	updated, _ = model.Update(AnimationTickMsg{Epoch: model.animationEpoch})
	model = updated.(Model)
	if frame := model.View(); !strings.Contains(frame, "⠙ Waiting for agent") {
		t.Fatalf("waiting spinner did not advance:\n%s", frame)
	}
	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision || after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("acknowledgement changed semantic publication: before=%+v after=%+v", before, after)
	}

	if err := eng.Apply(protocol.Operation{V: 1, Seq: 4, Op: protocol.OpProps, ID: "services", Props: json.RawMessage(`{"rows":[["billing","updated"]],"row_ids":["service:billing"]}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 5, Op: protocol.OpCommit, Frame: "updated"}); err != nil {
		t.Fatal(err)
	}
	updated, _ = model.Update(EngineDirtyMsg{})
	model = updated.(Model)
	if frame := model.View(); strings.Contains(frame, "Waiting for agent") || !strings.Contains(frame, "Enter Select") {
		t.Fatalf("next publication did not clear acknowledgement:\n%s", frame)
	}
}

func TestUXAcknowledgesSubmitAndDeclaredActionLocally(t *testing.T) {
	actions := a2runtime.NewActionRegistry(0, 1)
	if err := actions.Register("service.retry", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	eng := engine.New(protocol.DefaultLimits(), protocol.DefaultLimits().MaxPendingEvents, actions)
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "note", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"restart image worker","action":"note.submit"}`)},
		{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Retry","action":"service.retry"}]}`)},
		{V: 1, Seq: 3, Op: protocol.OpFocus, ID: "note"},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	model := NewModel(eng, DefaultTheme, nil)
	before := eng.PresentationSnapshot()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(Model)
	if ev, ok := eng.NextEvent(); !ok || ev.Ev != "submit" || ev.Value != "restart image worker" {
		t.Fatalf("submit=%+v ok=%v", ev, ok)
	}
	if _, ok := eng.NextEvent(); ok {
		t.Fatal("submit acknowledgement emitted a second semantic event")
	}
	if frame := model.View(); !strings.Contains(frame, "Submitted: restart image worker") || !strings.Contains(frame, "Waiting for agent") || strings.Contains(frame, "Enter Submit") {
		t.Fatalf("submit has no local acknowledgement:\n%s", frame)
	}
	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision || after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("submit acknowledgement changed semantic publication: before=%+v after=%+v", before, after)
	}

}

func TestUXAcknowledgesDeclaredActionLocally(t *testing.T) {
	actions := a2runtime.NewActionRegistry(0, 1)
	if err := actions.Register("service.retry", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	eng := engine.New(protocol.DefaultLimits(), protocol.DefaultLimits().MaxPendingEvents, actions)
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "services", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"rows":[["billing"]],"row_ids":["billing"]}`)},
		{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Retry","action":"service.retry"}]}`)},
		{V: 1, Seq: 3, Op: protocol.OpFocus, ID: "services"},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	model := NewModel(eng, DefaultTheme, nil)
	before := eng.PresentationSnapshot()
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	model = updated.(Model)
	if ev, ok := eng.NextEvent(); !ok || ev.Ev != "action_result" || ev.Action != "service.retry" {
		t.Fatalf("action=%+v ok=%v", ev, ok)
	}
	if _, ok := eng.NextEvent(); ok {
		t.Fatal("action acknowledgement emitted a second semantic event")
	}
	if frame := model.View(); !strings.Contains(frame, "Waiting for agent") {
		t.Fatalf("action has no local acknowledgement:\n%s", frame)
	}
	if frame := model.View(); !strings.Contains(frame, "Retry requested") {
		t.Fatalf("action has no local acceptance label:\n%s", frame)
	}
	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision || after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("action acknowledgement changed semantic publication: before=%+v after=%+v", before, after)
	}
}
