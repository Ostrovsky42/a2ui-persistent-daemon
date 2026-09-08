package bubbletea

import (
	"encoding/json"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTableNavigationSynchronizesDerivedWindowWithoutTableScrollKeys(t *testing.T) {
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
	assertTableWindowOffset(t, model, "agents", 10)

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnd})
	model = updated.(Model)
	selection, ok := eng.SelectedTableRow("agents")
	if !ok || selection.Index != 19 {
		t.Fatalf("End must update runtime selection, got %#v ok=%v", selection, ok)
	}
	assertTableWindowOffset(t, model, "agents", 16)

	// PageDown belongs to scrollable viewports, not tables. It must not create a
	// second table-scroll authority or move the runtime selection/window.
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	model = updated.(Model)
	selection, ok = eng.SelectedTableRow("agents")
	if !ok || selection.Index != 19 {
		t.Fatalf("PageDown unexpectedly changed table selection: %#v ok=%v", selection, ok)
	}
	assertTableWindowOffset(t, model, "agents", 16)
}
