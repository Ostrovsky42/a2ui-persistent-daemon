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
				{"title":"Task","width":28},
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
