package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"a2ui/engine"
	"a2ui/protocol"
)

type fakeController struct {
	snapshot        engine.PresentationSnapshot
	invokedID       string
	invokedAction   string
	invokedArgs     json.RawMessage
	selectedID      string
	selectedRowID   string
	ackedGeneration uint64
}

func (f *fakeController) Snapshot() engine.PresentationSnapshot { return f.snapshot }
func (f *fakeController) Focus(string) error                    { return nil }
func (f *fakeController) SetInput(string, string) error         { return nil }
func (f *fakeController) Submit(string) error                   { return nil }
func (f *fakeController) SelectTableRow(id, rowID string) error {
	f.selectedID, f.selectedRowID = id, rowID
	return nil
}
func (f *fakeController) ActivateTableSelection(string) error { return nil }
func (f *fakeController) InvokeAction(id, action string, args json.RawMessage) error {
	f.invokedID, f.invokedAction = id, action
	f.invokedArgs = append(json.RawMessage(nil), args...)
	return nil
}
func (f *fakeController) AcknowledgePublication(generation uint64) error {
	f.ackedGeneration = generation
	return nil
}

func semanticSnapshot(t *testing.T) engine.PresentationSnapshot {
	t.Helper()
	eng := engine.New(protocol.DefaultLimits(), 16, engine.NewNoopActions())
	ops := []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "title", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"Deploy approval"}`)},
		{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"y","label":"Approve","action":"approve","args":{"target":"prod"}}]}`)},
		{V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "jobs", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"columns":[{"title":"Job"}],"rows":[["Build"],["Deploy"]],"row_ids":["job-1","job-2"],"selectable":true}`)},
	}
	for _, op := range ops {
		if perr := eng.Apply(op); perr != nil {
			t.Fatal(perr)
		}
	}
	snap := eng.PresentationSnapshot()
	snap.PublicationGeneration = 7
	snap.PublicationPending = true
	return snap
}

func TestHandlerRendersSameSemanticSnapshotWithoutTerminalConcepts(t *testing.T) {
	ctrl := &fakeController{snapshot: semanticSnapshot(t)}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	NewHandler(ctrl).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"Deploy approval", "Approve", "Build", "job-2", "data-air-generation=\"7\""} {
		if !strings.Contains(body, want) {
			t.Fatalf("rendered page missing %q: %s", want, body)
		}
	}
	for _, forbidden := range []string{"action_key", "ArrowDown", "alternate screen"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("terminal concept %q leaked into web surface: %s", forbidden, body)
		}
	}
}

func TestHandlerInvokesSemanticActionWithoutPhysicalKey(t *testing.T) {
	ctrl := &fakeController{snapshot: semanticSnapshot(t)}
	form := url.Values{
		"type":   {"action_invoke"},
		"id":     {"actions"},
		"action": {"approve"},
		"args":   {`{"target":"prod"}`},
	}
	req := httptest.NewRequest(http.MethodPost, "/interaction", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	NewHandler(ctrl).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ctrl.invokedID != "actions" || ctrl.invokedAction != "approve" || string(ctrl.invokedArgs) != `{"target":"prod"}` {
		t.Fatalf("invocation id=%q action=%q args=%s", ctrl.invokedID, ctrl.invokedAction, ctrl.invokedArgs)
	}
}

func TestHandlerSelectsTableByStableRowID(t *testing.T) {
	ctrl := &fakeController{snapshot: semanticSnapshot(t)}
	form := url.Values{"type": {"table_select"}, "id": {"jobs"}, "row_id": {"job-2"}}
	req := httptest.NewRequest(http.MethodPost, "/interaction", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	NewHandler(ctrl).ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ctrl.selectedID != "jobs" || ctrl.selectedRowID != "job-2" {
		t.Fatalf("selection id=%q row_id=%q", ctrl.selectedID, ctrl.selectedRowID)
	}
}

func TestPublishedEndpointAcknowledgesExactGeneration(t *testing.T) {
	ctrl := &fakeController{snapshot: semanticSnapshot(t)}
	form := url.Values{"generation": {"7"}}
	req := httptest.NewRequest(http.MethodPost, "/published", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	NewHandler(ctrl).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ctrl.ackedGeneration != 7 {
		t.Fatalf("acked=%d", ctrl.ackedGeneration)
	}
}
