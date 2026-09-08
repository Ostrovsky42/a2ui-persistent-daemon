package bubbletea

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModelReconcilesTableWindowAsDerivedSelectionCache(t *testing.T) {
	eng := newTestEngine()
	rows, rowIDs := tableWindowFixture(20)
	props, err := json.Marshal(map[string]any{
		"selectable": true,
		"columns": []map[string]any{
			{"title": "Agent", "width": 14},
			{"title": "State", "width": 12},
		},
		"rows":    rows,
		"row_ids": rowIDs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "agents", Type: protocol.NodeTable, Parent: "root", Props: props}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.MoveTableSelection("agents", 13); perr != nil {
		t.Fatal(perr)
	}

	model := NewModelWithPreset(eng, DefaultTheme, PresetMinimal, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 10)

	// Growing the terminal keeps the current top row while the semantic
	// selection remains visible; resize does not create a second selection.
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 10)

	if perr := eng.MoveTableSelection("agents", 6); perr != nil {
		t.Fatal(perr)
	}
	updated, _ = model.Update(EngineDirtyMsg{})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 12)

	shortRows, shortIDs := tableWindowFixture(5)
	patch, err := json.Marshal(map[string]any{"rows": shortRows, "row_ids": shortIDs})
	if err != nil {
		t.Fatal(err)
	}
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "agents", Props: patch}); perr != nil {
		t.Fatal(perr)
	}
	updated, _ = model.Update(EngineDirtyMsg{})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 0)

	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 3, Op: protocol.OpRemove, ID: "agents"}); perr != nil {
		t.Fatal(perr)
	}
	updated, _ = model.Update(EngineDirtyMsg{})
	model = updated.(Model)
	if _, ok := model.interaction.TableViewports["agents"]; ok {
		t.Fatal("removed table retained adapter-local row-window cache")
	}
}

func TestTableWindowTracksStableRuntimeSelectionAcrossAgentReorder(t *testing.T) {
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
	if perr := eng.MoveTableSelection("agents", 13); perr != nil {
		t.Fatal(perr)
	}

	model := NewModelWithPreset(eng, DefaultTheme, PresetMinimal, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 6})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 10)

	selectedRow := rows[13]
	selectedID := rowIDs[13]
	reorderedRows := append([][]string(nil), rows...)
	reorderedIDs := append([]string(nil), rowIDs...)
	reorderedRows = append(reorderedRows[:13], reorderedRows[14:]...)
	reorderedIDs = append(reorderedIDs[:13], reorderedIDs[14:]...)
	reorderedRows = append(reorderedRows[:2], append([][]string{selectedRow}, reorderedRows[2:]...)...)
	reorderedIDs = append(reorderedIDs[:2], append([]string{selectedID}, reorderedIDs[2:]...)...)
	patch, _ := json.Marshal(map[string]any{"rows": reorderedRows, "row_ids": reorderedIDs})
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "agents", Props: patch}); perr != nil {
		t.Fatal(perr)
	}

	selection, ok := eng.SelectedTableRow("agents")
	if !ok || selection.RowID != selectedID || selection.Index != 2 {
		t.Fatalf("runtime selection did not follow stable row ID: %#v ok=%v", selection, ok)
	}
	updated, _ = model.Update(EngineDirtyMsg{})
	model = updated.(Model)
	assertTableWindowOffset(t, model, "agents", 2)
}

func tableWindowFixture(count int) ([][]string, []string) {
	rows := make([][]string, count)
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		rows[i] = []string{fmt.Sprintf("agent-%02d", i), "Running"}
		ids[i] = fmt.Sprintf("agent:%02d", i)
	}
	return rows, ids
}

func assertTableWindowOffset(t *testing.T, model Model, id string, want int) {
	t.Helper()
	state, ok := model.interaction.TableViewports[id]
	if !ok {
		t.Fatalf("missing adapter-local row-window cache for %q", id)
	}
	if state.Offset != want {
		t.Fatalf("table %q row-window offset: want %d, got %d", id, want, state.Offset)
	}
}
