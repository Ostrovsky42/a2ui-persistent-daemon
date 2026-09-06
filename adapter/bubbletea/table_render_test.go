package bubbletea

import (
	"encoding/json"
	"strings"
	"testing"

	"a2ui/protocol"
)

func TestRendererUsesMasterDetailAndPreservesSelectedRowFields(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "agents",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable": true,
			"columns": [
				{"title":"Agent","width":16},
				{"title":"State","width":14},
				{"title":"Attention","width":16},
				{"title":"Task","width":36},
				{"title":"Age","width":8}
			],
			"rows": [
				["Codex","Running","None","Index repository","4s"],
				["Claude","Waiting","Human","Review PR #438 and prepare merge","12s"]
			],
			"row_ids": ["agent:codex","agent:claude"]
		}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Focus("agents"); err != nil {
		t.Fatal(err)
	}
	if err := eng.MoveTableSelection("agents", 1); err != nil {
		t.Fatal(err)
	}

	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	frame := r.RenderTree(eng.Document(), "agents", eng.InputValues(), map[string]int{"agents": 1}, 80, 20)

	if !strings.Contains(frame, "Details") {
		t.Fatalf("expected master/detail presentation at width 80, got:\n%s", frame)
	}
	if !strings.Contains(frame, "Review PR #438 and prepare merge") {
		t.Fatalf("selected-row detail lost full Task value:\n%s", frame)
	}
	for _, field := range []string{"Claude", "Waiting", "Human", "12s"} {
		if !strings.Contains(frame, field) {
			t.Fatalf("selected-row detail lost field %q:\n%s", field, frame)
		}
	}
}

func TestRendererNarrowRecordFallbackPreservesAllFields(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "agents",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"columns": [
				{"title":"Agent","width":16},
				{"title":"State","width":14},
				{"title":"Task","width":28},
				{"title":"Age","width":8}
			],
			"rows": [["Codex","Running","Review adaptive table layout","4s"]]
		}`),
	}); err != nil {
		t.Fatal(err)
	}

	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	frame := r.RenderTree(eng.Document(), "", eng.InputValues(), nil, 20, 20)
	for _, field := range []string{"Agent", "Codex", "State", "Running", "Task", "Age", "4s"} {
		if !strings.Contains(frame, field) {
			t.Fatalf("record fallback lost field %q:\n%s", field, frame)
		}
	}
}

func TestRendererBoundsVisibleTableRowsByTerminalHeight(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "agents",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable": true,
			"columns": [
				{"title":"Agent","width":14},
				{"title":"State","width":12}
			],
			"rows": [
				["row-00","Running"],
				["row-01","Running"],
				["row-02","Running"],
				["row-03","Running"],
				["row-04","Running"],
				["row-05","Running"],
				["row-06","Running"],
				["row-07","Running"],
				["row-08","Running"],
				["row-09","Running"]
			],
			"row_ids": [
				"row:00","row:01","row:02","row:03","row:04",
				"row:05","row:06","row:07","row:08","row:09"
			]
		}`),
	}); err != nil {
		t.Fatal(err)
	}

	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	frame := r.RenderTree(eng.Document(), "agents", eng.InputValues(), map[string]int{"agents": 0}, 80, 6)

	for _, visible := range []string{"row-00", "row-01", "row-02", "row-03"} {
		if !strings.Contains(frame, visible) {
			t.Fatalf("expected %q in bounded table frame:\n%s", visible, frame)
		}
	}
	for _, hidden := range []string{"row-04", "row-05", "row-09"} {
		if strings.Contains(frame, hidden) {
			t.Fatalf("row %q leaked outside visible height window:\n%s", hidden, frame)
		}
	}
}

func TestRendererModeDoesNotFlapWhenOnlyRowContentLengthChanges(t *testing.T) {
	const width = 80
	columns := []tableColumn{
		{Title: "Agent", Width: 16},
		{Title: "State", Width: 14},
		{Title: "Attention", Width: 16},
		{Title: "Task", Width: 28},
		{Title: "Age", Width: 8},
	}
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  width,
		AvailableHeight: 20,
		Columns:         columns,
		RowCount:        1,
		Selectable:      true,
	})
	if plan.Mode != TableModeCompressed {
		t.Fatalf("test precondition: structural planner should choose compressed mode, got %v", plan.Mode)
	}

	eng := newTestEngine()
	applyRows := func(seq uint64, task string) {
		t.Helper()
		props, err := json.Marshal(map[string]any{
			"selectable": true,
			"columns": []map[string]any{
				{"title": "Agent", "width": 16},
				{"title": "State", "width": 14},
				{"title": "Attention", "width": 16},
				{"title": "Task", "width": 28},
				{"title": "Age", "width": 8},
			},
			"rows":    [][]string{{"worker-1", "ready", "normal", task, "1m"}},
			"row_ids": []string{"worker:1"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := eng.Apply(protocol.Operation{
			V:      1,
			Seq:    seq,
			Op:     protocol.OpUpsert,
			ID:     "agents",
			Type:   protocol.NodeTable,
			Parent: "root",
			Props:  props,
		}); err != nil {
			t.Fatal(err)
		}
	}

	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	render := func() string {
		return r.RenderTree(eng.Document(), "agents", eng.InputValues(), map[string]int{"agents": 0}, width, 20)
	}
	assertCompressedGrid := func(label, frame string) {
		t.Helper()
		if strings.Contains(frame, "Details") {
			t.Fatalf("%s row values switched renderer into master/detail:\n%s", label, frame)
		}
		if !strings.Contains(frame, " │ ") {
			t.Fatalf("%s renderer left tabular presentation unexpectedly:\n%s", label, frame)
		}
	}

	applyRows(1, "sync")
	shortFrame := render()
	assertCompressedGrid("short", shortFrame)

	applyRows(2, "this task description is deliberately longer than preferred and must not select another renderer mode")
	longFrame := render()
	assertCompressedGrid("long", longFrame)
}
