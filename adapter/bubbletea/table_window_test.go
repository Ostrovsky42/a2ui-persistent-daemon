package bubbletea

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"a2ui/protocol"
	a2runtime "a2ui/runtime"
)

func TestRendererHonorsLocalTableViewportOffset(t *testing.T) {
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
	if perr := eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "agents",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props:  props,
	}); perr != nil {
		t.Fatal(perr)
	}

	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	result := r.RenderFrame(
		eng.Document(),
		"agents",
		eng.InputValues(),
		map[string]a2runtime.TableSelection{"agents": {Index: 13, RowID: "row:13"}},
		InteractionState{TableViewports: map[string]TableViewportState{"agents": {Offset: 10}}},
		80,
		6,
		RenderState{CursorVisible: true},
	)

	for _, visible := range []string{"row-10", "row-11", "row-12", "row-13"} {
		if !strings.Contains(result.Frame, visible) {
			t.Fatalf("expected %q in preserved local window:\n%s", visible, result.Frame)
		}
	}
	for _, hidden := range []string{"row-09", "row-14"} {
		if strings.Contains(result.Frame, hidden) {
			t.Fatalf("unexpected %q outside preserved local window:\n%s", hidden, result.Frame)
		}
	}
}
