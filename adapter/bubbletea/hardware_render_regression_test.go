package bubbletea

import (
	"encoding/json"
	"testing"

	"a2ui/document"
	"github.com/charmbracelet/lipgloss"
)

func TestPreferredTableWidthDefaultsToVisibleTitleWidth(t *testing.T) {
	widths := preferredTableWidths([]tableColumn{{Title: "Surface"}})
	if len(widths) != 1 {
		t.Fatalf("width count=%d", len(widths))
	}
	wantMin := lipgloss.Width("Surface")
	if widths[0] < wantMin {
		t.Fatalf("default column width=%d want at least title width=%d", widths[0], wantMin)
	}
}

func TestInputConsumesAllocatedRendererWidth(t *testing.T) {
	r := NewRendererWithPreset(DefaultTheme, PresetDashboard)
	n := document.Node{ID: "answer", Props: map[string]json.RawMessage{"placeholder": json.RawMessage(`"type from-tui"`)}}
	const width = 40
	got := r.renderInput(n, true, "from-tui", len([]rune("from-tui")), width, RenderState{CursorVisible: true})
	if measured := lipgloss.Width(got); measured != width {
		t.Fatalf("rendered input width=%d want allocated width=%d\n%s", measured, width, got)
	}
}
