package bubbletea

import "testing"

func TestPlanTableLayoutUsesFullModeWhenPreferredWidthsFit(t *testing.T) {
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  80,
		AvailableHeight: 20,
		Columns: []tableColumn{
			{Title: "Agent", Width: 14},
			{Title: "State", Width: 12},
			{Title: "Task", Width: 24},
		},
		RowCount: 8,
	})

	if plan.Mode != TableModeFull {
		t.Fatalf("expected full mode, got %v", plan.Mode)
	}
	want := []int{14, 12, 24}
	if len(plan.ColumnWidths) != len(want) {
		t.Fatalf("expected %d column widths, got %d", len(want), len(plan.ColumnWidths))
	}
	for i := range want {
		if plan.ColumnWidths[i] != want[i] {
			t.Fatalf("column %d width: want %d, got %d", i, want[i], plan.ColumnWidths[i])
		}
	}
}

func TestPlanTableLayoutCompressesNonSelectableTableDeterministically(t *testing.T) {
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  25,
		AvailableHeight: 20,
		Columns: []tableColumn{
			{Title: "First", Width: 12},
			{Title: "Second", Width: 12},
			{Title: "Third", Width: 12},
		},
		RowCount: 8,
	})

	if plan.Mode != TableModeCompressed {
		t.Fatalf("expected compressed mode, got %v", plan.Mode)
	}
	if got := sum(plan.ColumnWidths) + 6; got > 25 {
		t.Fatalf("compressed table width %d exceeds available width 25", got)
	}
	for i, width := range plan.ColumnWidths {
		if width < 3 {
			t.Fatalf("column %d shrank below renderer minimum: %d", i, width)
		}
	}
}

func TestPlanTableLayoutUsesMasterDetailForSelectableMediumTable(t *testing.T) {
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  60,
		AvailableHeight: 20,
		Columns: []tableColumn{
			{Title: "Agent", Width: 16},
			{Title: "State", Width: 14},
			{Title: "Attention", Width: 16},
			{Title: "Task", Width: 28},
			{Title: "Age", Width: 8},
		},
		RowCount:    8,
		Selectable:  true,
		SelectedRow: 3,
	})

	if plan.Mode != TableModeMasterDetail {
		t.Fatalf("expected master/detail mode, got %v", plan.Mode)
	}
	if len(plan.VisibleCols) == 0 || len(plan.VisibleCols) >= 5 {
		t.Fatalf("expected a strict source-order prefix in master pane, got %v", plan.VisibleCols)
	}
	for i, col := range plan.VisibleCols {
		if col != i {
			t.Fatalf("master columns must be a source-order prefix, got %v", plan.VisibleCols)
		}
	}
	if plan.LeftPaneWidth < 1 || plan.RightPaneWidth < 1 {
		t.Fatalf("expected positive pane widths, got left=%d right=%d", plan.LeftPaneWidth, plan.RightPaneWidth)
	}
}

func TestPlanTableLayoutFallsBackToRecordsWhenNarrow(t *testing.T) {
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  20,
		AvailableHeight: 20,
		Columns: []tableColumn{
			{Title: "Agent", Width: 16},
			{Title: "State", Width: 14},
			{Title: "Attention", Width: 16},
			{Title: "Task", Width: 28},
		},
		RowCount:   8,
		Selectable: true,
	})

	if plan.Mode != TableModeRecords {
		t.Fatalf("expected records mode, got %v", plan.Mode)
	}
}
