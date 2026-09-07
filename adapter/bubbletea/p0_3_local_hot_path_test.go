package bubbletea

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"a2ui/engine"
	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func applyP03(t *testing.T, eng *engine.Engine, ops ...protocol.Operation) {
	t.Helper()
	for _, op := range ops {
		if perr := eng.Apply(op); perr != nil {
			t.Fatalf("apply %+v: %v", op, perr)
		}
	}
}

func p03WorkersTable(seq int64, id string) protocol.Operation {
	return protocol.Operation{
		V:      protocol.Version,
		Seq:    seq,
		Op:     protocol.OpUpsert,
		ID:     id,
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable":true,
			"action":"choose_worker",
			"columns":[{"title":"Worker","width":16}],
			"rows":[["worker-a"],["worker-b"],["worker-c"]],
			"row_ids":["worker-a","worker-b","worker-c"]
		}`),
	}
}

func pressP03(t *testing.T, model Model, key tea.KeyType) Model {
	t.Helper()
	updated, _ := model.Update(tea.KeyMsg{Type: key})
	return updated.(Model)
}

func assertP03NoEvents(t *testing.T, eng *engine.Engine) {
	t.Helper()
	if ev, ok := eng.NextEvent(); ok {
		t.Fatalf("unexpected semantic event: %+v", ev)
	}
}

func TestP03TableNavigationIsLocalAndEventFree(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng, p03WorkersTable(1, "workers"))
	if perr := eng.Focus("workers"); perr != nil {
		t.Fatal(perr)
	}
	before := eng.PresentationSnapshot()
	model := NewModel(eng, DefaultTheme, nil)

	steps := []struct {
		key  tea.KeyType
		want int
	}{
		{tea.KeyDown, 1},
		{tea.KeyDown, 2},
		{tea.KeyUp, 1},
		{tea.KeyEnd, 2},
		{tea.KeyHome, 0},
	}
	for _, step := range steps {
		model = pressP03(t, model, step.key)
		sel, ok := eng.SelectedTableRow("workers")
		if !ok || sel.Index != step.want {
			t.Fatalf("after %v selection=%+v ok=%v, want index %d", step.key, sel, ok, step.want)
		}
	}

	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision {
		t.Fatalf("navigation changed document revision: before=%d after=%d", before.Document.Revision, after.Document.Revision)
	}
	if after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("navigation changed publication generation: before=%d after=%d", before.PublicationGeneration, after.PublicationGeneration)
	}
	assertP03NoEvents(t, eng)
}

func TestP03TableEnterEmitsExactlyOneSelect(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng, p03WorkersTable(1, "workers"))
	if perr := eng.Focus("workers"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	model = pressP03(t, model, tea.KeyDown)
	assertP03NoEvents(t, eng)

	model = pressP03(t, model, tea.KeyEnter)
	ev, ok := eng.NextEvent()
	if !ok {
		t.Fatal("Enter emitted no select event")
	}
	if ev.Ev != "select" || ev.ID != "workers" || ev.RowID != "worker-b" || ev.Row != 1 {
		t.Fatalf("unexpected select event: %+v", ev)
	}
	assertP03NoEvents(t, eng)
}

func TestP03TablePageNavigationIsLocal(t *testing.T) {
	eng := newTestEngine()
	rows := make([][]string, 20)
	rowIDs := make([]string, 20)
	for i := range rows {
		rowIDs[i] = fmt.Sprintf("worker-%02d", i)
		rows[i] = []string{rowIDs[i]}
	}
	props, err := json.Marshal(map[string]any{
		"selectable": true,
		"columns":    []map[string]any{{"title": "Worker", "width": 16}},
		"rows":       rows,
		"row_ids":    rowIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	applyP03(t, eng, protocol.Operation{V: protocol.Version, Seq: 1, Op: protocol.OpUpsert, ID: "workers", Type: protocol.NodeTable, Parent: "root", Props: props})
	if perr := eng.Focus("workers"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	model.Width = 80
	model.Height = 8
	model.reconcileLocalState()

	model = pressP03(t, model, tea.KeyPgDown)
	sel, ok := eng.SelectedTableRow("workers")
	if !ok || sel.Index <= 0 {
		t.Fatalf("PgDown did not move by a local page: selection=%+v ok=%v", sel, ok)
	}
	assertP03NoEvents(t, eng)

	model = pressP03(t, model, tea.KeyPgUp)
	sel, ok = eng.SelectedTableRow("workers")
	if !ok || sel.Index != 0 {
		t.Fatalf("PgUp did not return to first page: selection=%+v ok=%v", sel, ok)
	}
	assertP03NoEvents(t, eng)
}

func TestP03MasterDetailSelectionUpdatesLocally(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng, protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "services",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable":true,
			"columns":[
				{"title":"Service","width":16},
				{"title":"State","width":14},
				{"title":"Attention","width":16},
				{"title":"Task","width":28},
				{"title":"Age","width":8}
			],
			"rows":[
				["service-a","ready","none","task-a-detail","1s"],
				["service-b","busy","high","task-b-detail","2s"]
			],
			"row_ids":["service-a","service-b"]
		}`),
	})
	if perr := eng.Focus("services"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	model.Width = 60
	model.Height = 20
	model.reconcileLocalState()
	before := eng.PresentationSnapshot()

	frameA := model.View()
	if !strings.Contains(frameA, "Details") || !strings.Contains(frameA, "task-a-detail") {
		t.Fatalf("initial master-detail frame missing service-a detail:\n%s", frameA)
	}
	model = pressP03(t, model, tea.KeyDown)
	frameB := model.View()
	if !strings.Contains(frameB, "task-b-detail") {
		t.Fatalf("local selection did not update detail pane:\n%s", frameB)
	}
	if frameA == frameB {
		t.Fatal("master-detail frame did not change after local selection move")
	}
	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision || after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("master-detail navigation mutated semantic publication state: before=%+v after=%+v", before, after)
	}
	assertP03NoEvents(t, eng)
}

func TestP03InputEditingStaysLocalUntilEnter(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng, protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "query",
		Type:   protocol.NodeInput,
		Parent: "root",
		Props:  json.RawMessage(`{"value":"abc","action":"search"}`),
	})
	if perr := eng.Focus("query"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	before := eng.PresentationSnapshot()

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	model = updated.(Model)
	for _, key := range []tea.KeyType{tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd, tea.KeyBackspace, tea.KeyDelete} {
		model = pressP03(t, model, key)
	}
	assertP03NoEvents(t, eng)
	afterEdit := eng.PresentationSnapshot()
	if afterEdit.Document.Revision != before.Document.Revision || afterEdit.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("input editing mutated document/publication state: before=%+v after=%+v", before, afterEdit)
	}

	model = pressP03(t, model, tea.KeyEnter)
	ev, ok := eng.NextEvent()
	if !ok || ev.Ev != "submit" || ev.ID != "query" {
		t.Fatalf("Enter submit event=%+v ok=%v", ev, ok)
	}
	assertP03NoEvents(t, eng)
}

func TestP03ViewportScrollingIsPurePresentation(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng,
		protocol.Operation{V: protocol.Version, Seq: 1, Op: protocol.OpUpsert, ID: "log", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"scrollable":true,"follow_tail":false}`)},
		protocol.Operation{V: protocol.Version, Seq: 2, Op: protocol.OpUpsert, ID: "log-text", Type: protocol.NodeText, Parent: "log", Props: json.RawMessage(`{"text":"01\n02\n03\n04\n05\n06\n07\n08\n09\n10\n11\n12"}`)},
	)
	if perr := eng.Focus("log"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	model.Width = 40
	model.Height = 6
	model.reconcileLocalState()
	before := eng.PresentationSnapshot()

	for _, key := range []tea.KeyType{tea.KeyDown, tea.KeyPgDown, tea.KeyUp, tea.KeyHome, tea.KeyEnd} {
		model = pressP03(t, model, key)
	}
	local := model.interaction.Viewports["log"]
	if local.Offset <= 0 {
		t.Fatalf("viewport End did not move to local max offset: %+v", local)
	}
	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision || after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("viewport scrolling mutated semantic publication state: before=%+v after=%+v", before, after)
	}
	assertP03NoEvents(t, eng)
}

func TestP03ResponsiveResizePreservesRowIdentityWithoutEvents(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng, protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "services",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable":true,
			"columns":[{"title":"Service","width":16},{"title":"State","width":14},{"title":"Attention","width":16},{"title":"Task","width":28},{"title":"Age","width":8}],
			"rows":[["service-a","ready","none","task-a","1s"],["service-b","busy","high","task-b","2s"]],
			"row_ids":["service-a","service-b"]
		}`),
	})
	if perr := eng.Focus("services"); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.SetTableSelection("services", 1); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	before := eng.PresentationSnapshot()

	model.Width = 80
	model.Height = 20
	model.reconcileLocalState()
	wide := model.View()
	model.Width = 20
	model.reconcileLocalState()
	narrow := model.View()
	model.Width = 80
	model.reconcileLocalState()
	wideAgain := model.View()
	if wide == narrow {
		t.Fatal("responsive table frame did not adapt between wide and narrow terminal")
	}
	if wideAgain != wide {
		t.Fatalf("wide -> narrow -> wide did not return to stable presentation\nfirst:\n%s\nagain:\n%s", wide, wideAgain)
	}
	sel, ok := eng.SelectedTableRow("services")
	if !ok || sel.RowID != "service-b" {
		t.Fatalf("resize lost stable selection identity: %+v ok=%v", sel, ok)
	}
	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision || after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("resize mutated semantic publication state: before=%+v after=%+v", before, after)
	}
	assertP03NoEvents(t, eng)
}

func TestP03StableRowIDSurvivesReorderAndDrivesDetail(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng, protocol.Operation{
		V:      protocol.Version,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "workers",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable":true,
			"columns":[{"title":"Worker","width":16},{"title":"State","width":14},{"title":"Task","width":28}],
			"rows":[["worker-a","ready","task-a"],["worker-b","busy","task-b-detail"],["worker-c","idle","task-c"]],
			"row_ids":["worker-a","worker-b","worker-c"]
		}`),
	})
	if perr := eng.Focus("workers"); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.SetTableSelection("workers", 1); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	model.Width = 45
	model.Height = 20
	model.reconcileLocalState()

	applyP03(t, eng, protocol.Operation{
		V:   protocol.Version,
		Seq: 2,
		Op:  protocol.OpProps,
		ID:  "workers",
		Props: json.RawMessage(`{
			"rows":[["worker-c","idle","task-c"],["worker-b","busy","task-b-detail"],["worker-a","ready","task-a"]],
			"row_ids":["worker-c","worker-b","worker-a"]
		}`),
	})
	model.reconcileLocalState()
	sel, ok := eng.SelectedTableRow("workers")
	if !ok || sel.RowID != "worker-b" || sel.Index != 1 {
		t.Fatalf("row reorder lost worker-b identity: %+v ok=%v", sel, ok)
	}
	if frame := model.View(); !strings.Contains(frame, "task-b-detail") {
		t.Fatalf("detail no longer follows worker-b after reorder:\n%s", frame)
	}
	assertP03NoEvents(t, eng)
}

func TestP03ContextFooterIsRendererOnlyAndFocusAware(t *testing.T) {
	eng := newTestEngine()
	applyP03(t, eng,
		p03WorkersTable(1, "workers"),
		protocol.Operation{V: protocol.Version, Seq: 2, Op: protocol.OpUpsert, ID: "query", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"","action":"search"}`)},
		protocol.Operation{V: protocol.Version, Seq: 3, Op: protocol.OpUpsert, ID: "log", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"scrollable":true}`)},
		protocol.Operation{V: protocol.Version, Seq: 4, Op: protocol.OpUpsert, ID: "log-text", Type: protocol.NodeText, Parent: "log", Props: json.RawMessage(`{"text":"one\ntwo\nthree\nfour\nfive"}`)},
		protocol.Operation{V: protocol.Version, Seq: 5, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Retry","action":"retry"},{"key":"down","label":"Shadow","action":"shadow"}]}`)},
	)
	before := eng.PresentationSnapshot()
	model := NewModel(eng, DefaultTheme, nil)
	model.Width = 100
	model.Height = 24

	if perr := eng.Focus("workers"); perr != nil {
		t.Fatal(perr)
	}
	model.reconcileLocalState()
	tableFrame := model.View()
	for _, hint := range []string{"↑↓ Move", "Home/End Jump", "Enter Select", "Tab Focus", "[R] Retry"} {
		if !strings.Contains(tableFrame, hint) {
			t.Fatalf("table footer missing %q:\n%s", hint, tableFrame)
		}
	}
	if strings.Contains(tableFrame, "Shadow") {
		t.Fatalf("footer advertised action shadowed by focused table navigation:\n%s", tableFrame)
	}
	if strings.Contains(tableFrame, "Esc") {
		t.Fatalf("footer invented unsupported Escape semantics:\n%s", tableFrame)
	}

	if perr := eng.Focus("query"); perr != nil {
		t.Fatal(perr)
	}
	model.reconcileLocalState()
	inputFrame := model.View()
	for _, hint := range []string{"←→ Cursor", "Enter Submit", "Tab Focus"} {
		if !strings.Contains(inputFrame, hint) {
			t.Fatalf("input footer missing %q:\n%s", hint, inputFrame)
		}
	}
	if strings.Contains(inputFrame, "[R] Retry") {
		t.Fatalf("footer advertised printable action while input owns printable keys:\n%s", inputFrame)
	}

	if perr := eng.Focus("log"); perr != nil {
		t.Fatal(perr)
	}
	model.reconcileLocalState()
	viewportFrame := model.View()
	for _, hint := range []string{"↑↓ Scroll", "PgUp/PgDn Page", "Home/End Edge", "Tab Focus", "[R] Retry"} {
		if !strings.Contains(viewportFrame, hint) {
			t.Fatalf("viewport footer missing %q:\n%s", hint, viewportFrame)
		}
	}

	after := eng.PresentationSnapshot()
	if after.Document.Revision != before.Document.Revision {
		t.Fatalf("footer changed Document revision: before=%d after=%d", before.Document.Revision, after.Document.Revision)
	}
	if after.PublicationGeneration != before.PublicationGeneration {
		t.Fatalf("footer changed publication generation: before=%d after=%d", before.PublicationGeneration, after.PublicationGeneration)
	}
	if model.PublishCount() != 0 {
		t.Fatalf("footer triggered renderer publication acknowledgement count=%d", model.PublishCount())
	}
	assertP03NoEvents(t, eng)
}
