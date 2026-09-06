package bubbletea

import "testing"

func TestPlanTableLayoutModeDoesNotFlapWhenOnlyRowContentLengthChanges(t *testing.T) {
	cols := []tableColumn{
		{Title: "Agent", Width: 16},
		{Title: "State", Width: 14},
		{Title: "Attention", Width: 16},
		{Title: "Task", Width: 28},
		{Title: "Age", Width: 8},
	}
	shortRows := [][]string{{"worker-1", "ready", "normal", "sync", "1m"}}
	longRows := [][]string{{"worker-1", "ready", "normal", "this task description is deliberately longer than preferred", "1m"}}

	shortPlan := planTableLayout(TableLayoutInput{
		AvailableWidth:  80,
		AvailableHeight: 20,
		Columns:         cols,
		Rows:            shortRows,
		RowCount:        len(shortRows),
		Selectable:      true,
	})
	if shortPlan.Mode != TableModeCompressed {
		t.Fatalf("test precondition: short-row dataset should fit compressed mode, got %v", shortPlan.Mode)
	}

	longPlan := planTableLayout(TableLayoutInput{
		AvailableWidth:  80,
		AvailableHeight: 20,
		Columns:         cols,
		Rows:            longRows,
		RowCount:        len(longRows),
		Selectable:      true,
	})
	if longPlan.Mode != shortPlan.Mode {
		t.Fatalf("row content length changed presentation mode at fixed schema/geometry: short=%v long=%v", shortPlan.Mode, longPlan.Mode)
	}
	if len(longPlan.ReadableWidths) != len(shortPlan.ReadableWidths) {
		t.Fatalf("readable floor count changed: short=%v long=%v", shortPlan.ReadableWidths, longPlan.ReadableWidths)
	}
	for i := range shortPlan.ReadableWidths {
		if longPlan.ReadableWidths[i] != shortPlan.ReadableWidths[i] {
			t.Fatalf("row content changed readable floor for column %d: short=%d long=%d", i, shortPlan.ReadableWidths[i], longPlan.ReadableWidths[i])
		}
	}
}
