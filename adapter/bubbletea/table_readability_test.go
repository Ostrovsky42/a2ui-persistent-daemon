package bubbletea

import "testing"

func TestMinimumReadableTableWidthsUseStableColumnContract(t *testing.T) {
	cols := agentActivityTableColumns()
	rows := agentActivityTableRows()

	got := minimumReadableTableWidths(cols, rows)
	want := []int{12, 11, 12, 27, 6}
	if len(got) != len(want) {
		t.Fatalf("readable width count: want %d, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("readable width[%d]: want %d, got %d (all=%v)", i, want[i], got[i], got)
		}
	}

	rows[0][3] = "a streamed task value much longer than the declared preferred width"
	afterRowChange := minimumReadableTableWidths(cols, rows)
	for i := range got {
		if afterRowChange[i] != got[i] {
			t.Fatalf("row content changed structural readable floor[%d]: before=%d after=%d", i, got[i], afterRowChange[i])
		}
	}
}

func TestPlanTableLayoutCompressesWithoutCrossingReadableFloors(t *testing.T) {
	cols := agentActivityTableColumns()
	rows := agentActivityTableRows()
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  90,
		AvailableHeight: 20,
		Columns:         cols,
		Rows:            rows,
		RowCount:        len(rows),
		Selectable:      true,
		SelectedRow:     1,
	})

	if plan.Mode != TableModeCompressed {
		t.Fatalf("expected compressed mode while every column can remain readable, got %v", plan.Mode)
	}
	floors := minimumReadableTableWidths(cols, rows)
	for i, width := range plan.ColumnWidths {
		if width < floors[i] {
			t.Fatalf("column %d crossed readable floor: width=%d floor=%d", i, width, floors[i])
		}
	}
}

func TestPlanTableLayoutSwitchesToMasterDetailBeforeReadableFloorViolation(t *testing.T) {
	cols := agentActivityTableColumns()
	rows := agentActivityTableRows()
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  80,
		AvailableHeight: 20,
		Columns:         cols,
		Rows:            rows,
		RowCount:        len(rows),
		Selectable:      true,
		SelectedRow:     1,
	})

	if plan.Mode != TableModeMasterDetail {
		t.Fatalf("expected master/detail once structural tabular floors no longer fit, got %v", plan.Mode)
	}
}
