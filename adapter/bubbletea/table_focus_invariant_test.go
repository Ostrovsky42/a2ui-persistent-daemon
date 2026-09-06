package bubbletea

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAdaptiveTablePresentationDoesNotChangeRuntimeFocusOrder(t *testing.T) {
	eng := newTestEngine()
	ops := []protocol.Operation{
		{
			V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "agents", Type: protocol.NodeTable, Parent: "root",
			Props: json.RawMessage(`{"selectable":true,"columns":[{"title":"Agent","width":16},{"title":"State","width":14},{"title":"Attention","width":16},{"title":"Task","width":28},{"title":"Age","width":8}],"rows":[["Codex","Running","None","Index repository","4s"],["Claude","Waiting","Human","Review PR #438 and prepare merge","12s"]]}`),
		},
		{
			V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "command", Type: protocol.NodeInput, Parent: "root",
			Props: json.RawMessage(`{"placeholder":"command"}`),
		},
		{
			V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "log", Type: protocol.NodeViewport, Parent: "root",
			Props: json.RawMessage(`{"height":5,"scrollable":true}`),
		},
	}
	for _, op := range ops {
		if perr := eng.Apply(op); perr != nil {
			t.Fatal(perr)
		}
	}
	if perr := eng.Focus("agents"); perr != nil {
		t.Fatal(perr)
	}

	model := NewModelWithPreset(eng, DefaultTheme, PresetMinimal, nil)
	assertCycle := func(width int) {
		t.Helper()
		updated, _ := model.Update(tea.WindowSizeMsg{Width: width, Height: 20})
		model = updated.(Model)
		_ = model.View() // force the width-specific presentation path

		want := []string{"command", "log", "agents"}
		for _, id := range want {
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
			model = updated.(Model)
			if got := eng.FocusedID(); got != id {
				t.Fatalf("width %d changed Tab order: want focus %q, got %q", width, id, got)
			}
		}
	}

	assertCycle(120) // full
	assertCycle(80)  // master/detail
	assertCycle(20)  // records
}
