package bubbletea

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

type fakeSemanticController struct {
	snapshot      engine.PresentationSnapshot
	setID         string
	setValue      string
	invokedID     string
	invokedAction string
	invokedArgs   json.RawMessage
	acks          []uint64
	connErr       error
}

func (f *fakeSemanticController) Snapshot() engine.PresentationSnapshot { return f.snapshot }
func (f *fakeSemanticController) ConnectionError() error                { return f.connErr }
func (f *fakeSemanticController) Focus(string) error                    { return nil }
func (f *fakeSemanticController) SetInput(id, value string) error {
	f.setID, f.setValue = id, value
	f.snapshot.InputValues[id] = value
	return nil
}
func (f *fakeSemanticController) Submit(string) error                  { return nil }
func (f *fakeSemanticController) MoveTableSelection(string, int) error { return nil }
func (f *fakeSemanticController) ActivateTableSelection(string) error  { return nil }
func (f *fakeSemanticController) InvokeAction(id, action string, args json.RawMessage) error {
	f.invokedID = id
	f.invokedAction = action
	f.invokedArgs = append(json.RawMessage(nil), args...)
	return nil
}
func (f *fakeSemanticController) AcknowledgePublication(generation uint64) error {
	f.acks = append(f.acks, generation)
	return nil
}

func TestModelCanRenderAndEditThroughSemanticControllerWithoutEngine(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "input", Type: protocol.NodeInput, Parent: "root",
		Props: json.RawMessage(`{"value":"a"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Focus("input"); err != nil {
		t.Fatal(err)
	}
	ctrl := &fakeSemanticController{snapshot: eng.PresentationSnapshot()}

	m := NewModelWithController(ctrl, DefaultTheme, PresetMinimal, nil)
	if m.Engine != nil {
		t.Fatal("remote/controller-backed model must not require a local authoritative Engine")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Ж")})
	m = updated.(Model)
	if ctrl.setID != "input" || ctrl.setValue != "aЖ" {
		t.Fatalf("controller SetInput got id=%q value=%q", ctrl.setID, ctrl.setValue)
	}
	if got := m.View(); got == "" {
		t.Fatal("controller-backed model rendered an empty frame")
	}
}

func TestPhysicalActionKeyIsResolvedToSemanticInvocationInsideRenderer(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root",
		Props: json.RawMessage(`{"items":[{"key":"y","label":"Approve","action":"approve","args":{"target":"prod"}}]}`),
	}); err != nil {
		t.Fatal(err)
	}
	ctrl := &fakeSemanticController{snapshot: eng.PresentationSnapshot()}
	m := NewModelWithController(ctrl, DefaultTheme, PresetMinimal, nil)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	_ = updated.(Model)
	if ctrl.invokedID != "actions" || ctrl.invokedAction != "approve" || string(ctrl.invokedArgs) != `{"target":"prod"}` {
		t.Fatalf("semantic invocation id=%q action=%q args=%s", ctrl.invokedID, ctrl.invokedAction, ctrl.invokedArgs)
	}
}

func TestViewAcknowledgesExactSnapshotPublicationGenerationOnce(t *testing.T) {
	eng := newTestEngine()
	snapshot := eng.PresentationSnapshot()
	snapshot.PublicationGeneration = 17
	snapshot.PublicationPending = true
	ctrl := &fakeSemanticController{snapshot: snapshot}
	m := NewModelWithController(ctrl, DefaultTheme, PresetMinimal, nil)

	_ = m.View()
	_ = m.View()
	if len(ctrl.acks) != 1 || ctrl.acks[0] != 17 {
		t.Fatalf("publication acknowledgements = %#v, want exactly [17]", ctrl.acks)
	}
	if got := m.PublishCount(); got != 1 {
		t.Fatalf("PublishCount=%d, want 1", got)
	}
}

func TestViewSurfacesControllerDisconnectWithoutForkingLocalEngine(t *testing.T) {
	eng := newTestEngine()
	ctrl := &fakeSemanticController{snapshot: eng.PresentationSnapshot(), connErr: errors.New("socket closed")}
	m := NewModelWithController(ctrl, DefaultTheme, PresetMinimal, nil)
	view := m.View()
	if !strings.Contains(view, "Daemon disconnected") || !strings.Contains(view, "socket closed") {
		t.Fatalf("disconnect state missing from frame: %q", view)
	}
	if m.Engine != nil {
		t.Fatal("disconnected remote client must not fall back to a local Engine")
	}
}

func TestViewDoesNotTreatRecoverableControllerDiagnosticAsDisconnect(t *testing.T) {
	eng := newTestEngine()
	ctrl := &fakeSemanticController{snapshot: eng.PresentationSnapshot()}
	m := NewModelWithController(ctrl, DefaultTheme, PresetMinimal, nil)
	view := m.View()
	if strings.Contains(view, "Daemon disconnected") {
		t.Fatalf("healthy controller rendered disconnect state: %q", view)
	}
}
