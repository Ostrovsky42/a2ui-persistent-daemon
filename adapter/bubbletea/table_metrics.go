package bubbletea

import (
	"encoding/json"

	"a2ui/document"
	a2runtime "a2ui/runtime"
)

func resolveTableViewportMetrics(n document.Node, selection a2runtime.TableSelection, maxW, maxH, rowOffset int) TableViewportMetrics {
	var cols []tableColumn
	var rows [][]string
	_ = json.Unmarshal(n.Props["columns"], &cols)
	_ = json.Unmarshal(n.Props["rows"], &rows)
	for i := range cols {
		cols[i].Title = SanitizeSingleLineText(cols[i].Title)
	}
	for i := range rows {
		for j := range rows[i] {
			rows[i][j] = SanitizeSingleLineText(rows[i][j])
		}
	}
	selectedRow := selection.Index
	if selectedRow < 0 {
		selectedRow = 0
	}
	if len(rows) > 0 && selectedRow >= len(rows) {
		selectedRow = len(rows) - 1
	}
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  maxW,
		AvailableHeight: maxH,
		Columns:         cols,
		RowCount:        len(rows),
		Selectable:      propBool(n, "selectable", false),
		SelectedRow:     selectedRow,
		RowOffset:       rowOffset,
		Variant:         propString(n, "variant", "normal"),
	})
	visible := plan.RowEnd - plan.RowStart
	if visible < 0 {
		visible = 0
	}
	maxOffset := len(rows) - visible
	if maxOffset < 0 {
		maxOffset = 0
	}
	return TableViewportMetrics{
		TotalRows:   len(rows),
		VisibleRows: visible,
		MaxOffset:   maxOffset,
		Offset:      plan.RowStart,
	}
}
