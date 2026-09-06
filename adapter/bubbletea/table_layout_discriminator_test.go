package bubbletea

import "testing"

func agentActivityTableColumns() []tableColumn {
	return []tableColumn{
		{Title: "Agent", Width: 16},
		{Title: "State", Width: 14},
		{Title: "Attention", Width: 16},
		{Title: "Task", Width: 36},
		{Title: "Age", Width: 8},
	}
}

func agentActivityTableRows() [][]string {
	return [][]string{
		{"Codex", "Running", "None", "Index repository", "4s"},
		{"Claude", "Waiting", "Human", "Review PR #438 and prepare merge", "12s"},
	}
}

func TestPlanTableLayoutAgentDatasetUsesMasterDetailAtNarrowGeometry(t *testing.T) {
	rows := agentActivityTableRows()
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  80,
		AvailableHeight: 20,
		Columns:         agentActivityTableColumns(),
		RowCount:        len(rows),
		Selectable:      true,
		SelectedRow:     1,
	})

	if plan.Mode != TableModeMasterDetail {
		t.Fatalf("planner discriminator: width 80 must choose master/detail for the agent table contract, got %v", plan.Mode)
	}
}

func TestPlanTableLayoutAgentDatasetUsesFullWhenPreferredWidthsFit(t *testing.T) {
	rows := agentActivityTableRows()
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  120,
		AvailableHeight: 20,
		Columns:         agentActivityTableColumns(),
		RowCount:        len(rows),
		Selectable:      true,
		SelectedRow:     1,
	})

	if plan.Mode != TableModeFull {
		t.Fatalf("planner discriminator: wide geometry must preserve full mode for the same table contract, got %v", plan.Mode)
	}
}
