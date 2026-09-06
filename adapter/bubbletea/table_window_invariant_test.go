package bubbletea

import (
	"encoding/json"
	"fmt"
	"testing"

	"a2ui/engine"
	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

type tableWindowInvariantFixture struct {
	eng    *engine.Engine
	model  Model
	rows   [][]string
	rowIDs []string
}

func newTableWindowInvariantFixture(t *testing.T) tableWindowInvariantFixture {
	t.Helper()
	eng := newTestEngine()
	rows := make([][]string, 20)
	rowIDs := make([]string, 20)
	for i := range rows {
		rows[i] = []string{fmt.Sprintf("row-%02d", i), "Running"}
		rowIDs[i] = fmt.Sprintf("row:%02d", i)
	}
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
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "note", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":""}`)}); perr != nil {
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
	assertTableWindowFixedPoint(t, model, "agents")
	return tableWindowInvariantFixture{eng: eng, model: model, rows: rows, rowIDs: rowIDs}
}

func assertTableWindowFixedPoint(t *testing.T, model Model, id string) {
	t.Helper()
	local, ok := model.interaction.TableViewports[id]
	if !ok {
		t.Fatalf("missing local table window state for %q", id)
	}
	result := model.renderResult()
	metrics, ok := result.Tables[id]
	if !ok {
		t.Fatalf("renderer did not return table metrics for %q", id)
	}
	if local.Offset != metrics.Offset {
		t.Fatalf("stale table window cache for %q: cached=%d renderer_resolved=%d", id, local.Offset, metrics.Offset)
	}
}

func updateTableRows(t *testing.T, eng *engine.Engine, seq int64, rows [][]string, rowIDs []string) {
	t.Helper()
	props, err := json.Marshal(map[string]any{"rows": rows, "row_ids": rowIDs})
	if err != nil {
		t.Fatal(err)
	}
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: seq, Op: protocol.OpProps, ID: "agents", Props: props}); perr != nil {
		t.Fatal(perr)
	}
}

func reconcileAfterEngineMutation(model Model) Model {
	updated, _ := model.Update(EngineDirtyMsg{})
	return updated.(Model)
}

func TestTableWindowContinuityStateIsReconciledAfterEveryMutationClass(t *testing.T) {
	t.Run("selection navigation", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		updated, _ := f.model.Update(tea.KeyMsg{Type: tea.KeyEnd})
		model := updated.(Model)
		selection, ok := f.eng.SelectedTableRow("agents")
		if !ok || selection.Index != 19 {
			t.Fatalf("End selection=%#v ok=%v", selection, ok)
		}
		assertTableWindowFixedPoint(t, model, "agents")
	})

	t.Run("resize", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		updated, _ := f.model.Update(tea.WindowSizeMsg{Width: 80, Height: 5})
		assertTableWindowFixedPoint(t, updated.(Model), "agents")
	})

	t.Run("focus change", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		before := f.model.interaction.TableViewports["agents"].Offset
		if perr := f.eng.Focus("note"); perr != nil {
			t.Fatal(perr)
		}
		model := reconcileAfterEngineMutation(f.model)
		assertTableWindowFixedPoint(t, model, "agents")
		if after := model.interaction.TableViewports["agents"].Offset; after != before {
			t.Fatalf("focus-only mutation moved valid table window: before=%d after=%d", before, after)
		}
	})

	t.Run("stable row id reorder", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		rows := append([][]string(nil), f.rows...)
		ids := append([]string(nil), f.rowIDs...)
		selectedRow, selectedID := rows[13], ids[13]
		rows = append(rows[:13], rows[14:]...)
		ids = append(ids[:13], ids[14:]...)
		rows = append([][]string{rows[0], selectedRow}, rows[1:]...)
		ids = append([]string{ids[0], selectedID}, ids[1:]...)
		updateTableRows(t, f.eng, 3, rows, ids)
		model := reconcileAfterEngineMutation(f.model)
		selection, ok := f.eng.SelectedTableRow("agents")
		if !ok || selection.RowID != "row:13" || selection.Index != 1 {
			t.Fatalf("stable row-id selection not preserved: %#v ok=%v", selection, ok)
		}
		assertTableWindowFixedPoint(t, model, "agents")
	})

	t.Run("row shrink", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		updateTableRows(t, f.eng, 3, f.rows[:8], f.rowIDs[:8])
		model := reconcileAfterEngineMutation(f.model)
		selection, ok := f.eng.SelectedTableRow("agents")
		if !ok || selection.Index < 0 || selection.Index >= 8 {
			t.Fatalf("selection not reconciled after shrink: %#v ok=%v", selection, ok)
		}
		assertTableWindowFixedPoint(t, model, "agents")
	})

	t.Run("non-selectable prunes state", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		if perr := f.eng.Apply(protocol.Operation{V: 1, Seq: 3, Op: protocol.OpProps, ID: "agents", Props: json.RawMessage(`{"selectable":false}`)}); perr != nil {
			t.Fatal(perr)
		}
		model := reconcileAfterEngineMutation(f.model)
		if _, ok := model.interaction.TableViewports["agents"]; ok {
			t.Fatal("non-selectable table retained local table window state")
		}
	})

	t.Run("remove prunes state", func(t *testing.T) {
		f := newTableWindowInvariantFixture(t)
		if perr := f.eng.Apply(protocol.Operation{V: 1, Seq: 3, Op: protocol.OpRemove, ID: "agents"}); perr != nil {
			t.Fatal(perr)
		}
		model := reconcileAfterEngineMutation(f.model)
		if _, ok := model.interaction.TableViewports["agents"]; ok {
			t.Fatal("removed table retained local table window state")
		}
	})
}
