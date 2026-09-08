package bubbletea

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTableNavigationSynchronizesDerivedWindowWithoutTableScrollAuthority(t *testing.T) {
	eng := newTestEngine()
	rows, rowIDs := tableWindowFixture(20)
	props, _ := json.Marshal(map[string]any{
		"selectable": true,
		"columns":    []map[string]any{{"title": "Agent", "width": 14}, {"title": "State", "width": 12}},
		"rows":       rows, "row_ids": rowIDs,
	})
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "agents", Type: protocol.NodeTable, Parent: "root", Props: props}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.Focus("agents"); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.MoveTableSelection("agents", 13); perr != nil {
		t.Fatal(perr)
	}

	model := NewModelWithPreset(eng, DefaultTheme, PresetMinimal, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 11)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = updated.(Model)
	selection, ok := eng.SelectedTableRow("agents")
	if !ok || selection.Index != 19 {
		t.Fatalf("End must update runtime selection, got %#v ok=%v", selection, ok)
	}
	assertTableWindowOffset(t, model, "agents", 17)

	// PageDown is also table-selection navigation in P0.3. At the end it clamps
	// locally and must neither invent an independent table-scroll authority nor
	// emit a semantic event.
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	selection, ok = eng.SelectedTableRow("agents")
	if !ok || selection.Index != 19 {
		t.Fatalf("PageDown at end must clamp selection, got %#v ok=%v", selection, ok)
	}
	assertTableWindowOffset(t, model, "agents", 17)
	if ev, ok := eng.NextEvent(); ok {
		t.Fatalf("local table page navigation emitted semantic event: %+v", ev)
	}
}
