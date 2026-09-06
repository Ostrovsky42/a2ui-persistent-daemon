package bubbletea

import (
	"encoding/json"
	"strings"

	"a2ui/document"
	a2runtime "a2ui/runtime"
	"github.com/charmbracelet/lipgloss"
)

// renderTableWindowed computes one structural plan from the complete column
// contract plus row count, then applies renderer-local row windowing to that
// plan. It never re-plans a projected row slice, so presentation mode and
// readable-width decisions are stable across vertical windowing and row values.
func (r *Renderer) renderTableWindowed(n document.Node, focused bool, selection a2runtime.TableSelection, maxW, maxH, rowOffset int) string {
	cols, rows := decodeSanitizedTable(n)
	if len(cols) == 0 {
		return ""
	}

	selectedRow := selection.Index
	if selectedRow < 0 {
		selectedRow = 0
	}
	if len(rows) > 0 && selectedRow >= len(rows) {
		selectedRow = len(rows) - 1
	}
	selectable := propBool(n, "selectable", false)
	variant := propString(n, "variant", "normal")
	plan := planTableLayout(TableLayoutInput{
		AvailableWidth:  maxW,
		AvailableHeight: maxH,
		Columns:         cols,
		RowCount:        len(rows),
		Selectable:      selectable,
		SelectedRow:     selectedRow,
		RowOffset:       rowOffset,
		Variant:         variant,
	})

	visibleRows := rows
	visibleSelection := selectedRow
	if plan.RowStart >= 0 && plan.RowEnd >= plan.RowStart && plan.RowEnd <= len(rows) && (plan.RowStart != 0 || plan.RowEnd != len(rows)) {
		visibleRows = rows[plan.RowStart:plan.RowEnd]
		visibleSelection = selectedRow - plan.RowStart
	}

	switch plan.Mode {
	case TableModeRecords:
		return r.renderTableRecords(cols, visibleRows, focused, selectable, visibleSelection, maxW)
	case TableModeMasterDetail:
		return r.renderPlannedTableMasterDetail(cols, visibleRows, focused, selectable, visibleSelection, variant, plan)
	default:
		return r.renderTableGrid(cols, visibleRows, focused, selectable, visibleSelection, variant, plan.ColumnWidths)
	}
}

func decodeSanitizedTable(n document.Node) ([]tableColumn, [][]string) {
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
	return cols, rows
}

func (r *Renderer) renderPlannedTableMasterDetail(cols []tableColumn, rows [][]string, focused, selectable bool, selectedRow int, variant string, plan TableLayoutPlan) string {
	if len(rows) == 0 || selectedRow < 0 || selectedRow >= len(rows) {
		return r.renderTableRecords(cols, rows, focused, selectable, selectedRow, plan.LeftPaneWidth+tablePaneSeparatorWidth+plan.RightPaneWidth)
	}

	masterCols := make([]tableColumn, 0, len(plan.VisibleCols))
	masterRows := make([][]string, len(rows))
	masterPreferred := make([]int, 0, len(plan.VisibleCols))
	masterFloors := make([]int, 0, len(plan.VisibleCols))
	preferred := preferredTableWidths(cols)
	for _, columnIndex := range plan.VisibleCols {
		if columnIndex < 0 || columnIndex >= len(cols) {
			continue
		}
		masterCols = append(masterCols, cols[columnIndex])
		masterPreferred = append(masterPreferred, preferred[columnIndex])
		floor := tableMinColumnWidth
		if columnIndex < len(plan.ReadableWidths) {
			floor = plan.ReadableWidths[columnIndex]
		}
		masterFloors = append(masterFloors, floor)
		for rowIndex, row := range rows {
			value := ""
			if columnIndex < len(row) {
				value = row[columnIndex]
			}
			masterRows[rowIndex] = append(masterRows[rowIndex], value)
		}
	}
	if len(masterCols) == 0 {
		return r.renderTableRecords(cols, rows, focused, selectable, selectedRow, plan.LeftPaneWidth+tablePaneSeparatorWidth+plan.RightPaneWidth)
	}

	prefixW := 0
	if selectable {
		prefixW = tableSelectablePrefixW
	}
	separatorW := tableColumnSeparatorWidth(variant) * maxInt(len(masterCols)-1, 0)
	budget := maxInt(plan.LeftPaneWidth-prefixW-separatorW, sumTableWidths(masterFloors))
	masterWidths := shrinkTableWidthsToFloors(masterPreferred, masterFloors, budget)
	master := r.renderTableGrid(masterCols, masterRows, focused, selectable, selectedRow, variant, masterWidths)
	detail := r.renderTableDetail(cols, rows[selectedRow], plan.RightPaneWidth)
	return lipgloss.JoinHorizontal(lipgloss.Top, master, strings.Repeat(" ", tablePaneSeparatorWidth), detail)
}
