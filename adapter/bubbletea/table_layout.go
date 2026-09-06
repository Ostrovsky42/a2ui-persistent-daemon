package bubbletea

import "github.com/charmbracelet/lipgloss"

// TablePresentationMode is renderer-local presentation state. It is not part
// of the A2UI wire protocol or authoritative Document.
type TablePresentationMode int

const (
	TableModeFull TablePresentationMode = iota
	TableModeCompressed
	TableModeMasterDetail
	TableModeRecords
)

const (
	tableMinColumnWidth            = 3
	tableSelectablePrefixW         = 2
	tableNormalSeparatorW          = 3
	tableCompactSeparatorW         = 1
	tablePaneSeparatorWidth        = 3
	tableReadablePreferredNumerator   = 3
	tableReadablePreferredDenominator = 4
)

// TableLayoutInput contains only deterministic renderer inputs. Semantic
// selection remains runtime-owned; SelectedRow is read-only input here.
type TableLayoutInput struct {
	AvailableWidth  int
	AvailableHeight int
	Columns         []tableColumn
	Rows            [][]string
	RowCount        int
	Selectable      bool
	SelectedRow     int
	RowOffset       int
	Variant         string
}

// TableLayoutPlan is an ephemeral renderer plan. Applying it must never mutate
// the Document, runtime semantic state, or publication generation.
type TableLayoutPlan struct {
	Mode           TablePresentationMode
	ColumnWidths   []int
	ReadableWidths []int
	VisibleCols    []int
	LeftPaneWidth  int
	RightPaneWidth int
	RowStart       int
	RowEnd         int
}

func planTableLayout(in TableLayoutInput) TableLayoutPlan {
	widths := preferredTableWidths(in.Columns)
	readable := minimumReadableTableWidths(in.Columns, in.Rows)
	plan := TableLayoutPlan{
		Mode:           TableModeFull,
		ColumnWidths:   append([]int(nil), widths...),
		ReadableWidths: append([]int(nil), readable...),
		VisibleCols:    allColumnIndexes(len(in.Columns)),
	}
	if len(in.Columns) == 0 || in.AvailableWidth <= 0 {
		return finishTablePlan(in, plan)
	}

	prefixW := 0
	if in.Selectable {
		prefixW = tableSelectablePrefixW
	}
	separatorW := tableColumnSeparatorWidth(in.Variant)
	separatorTotal := separatorW * maxInt(len(in.Columns)-1, 0)
	preferredTotal := prefixW + separatorTotal + sumTableWidths(widths)
	if preferredTotal <= in.AvailableWidth {
		return finishTablePlan(in, plan)
	}

	readableTotal := prefixW + separatorTotal + sumTableWidths(readable)
	if readableTotal <= in.AvailableWidth {
		plan.Mode = TableModeCompressed
		plan.ColumnWidths = shrinkTableWidthsToFloors(widths, readable, in.AvailableWidth-prefixW-separatorTotal)
		return finishTablePlan(in, plan)
	}

	if masterDetailEligible(in, readable) {
		left, right := masterDetailPaneWidths(in, readable)
		plan.Mode = TableModeMasterDetail
		plan.LeftPaneWidth = left
		plan.RightPaneWidth = right
		plan.VisibleCols = masterColumnPrefix(readable, left, prefixW, separatorW)
		return finishTablePlan(in, plan)
	}

	plan.Mode = TableModeRecords
	return finishTablePlan(in, plan)
}

func finishTablePlan(in TableLayoutInput, plan TableLayoutPlan) TableLayoutPlan {
	capacity := tableVisibleRowCapacity(in, plan.Mode)
	plan.RowStart, plan.RowEnd = visibleTableRowWindow(in.RowCount, in.SelectedRow, in.RowOffset, capacity)
	return plan
}

func tableVisibleRowCapacity(in TableLayoutInput, mode TablePresentationMode) int {
	if in.RowCount <= 0 {
		return 0
	}
	height := in.AvailableHeight
	if height < 1 {
		height = 1
	}

	if mode == TableModeRecords {
		perRecord := len(in.Columns) + 1
		if perRecord < 1 {
			perRecord = 1
		}
		capacity := height / perRecord
		if capacity < 1 {
			capacity = 1
		}
		if capacity > in.RowCount {
			capacity = in.RowCount
		}
		return capacity
	}

	chrome := 2
	if in.Variant == "dense" {
		chrome = 1
	}
	capacity := height - chrome
	if capacity < 1 {
		capacity = 1
	}
	if capacity > in.RowCount {
		capacity = in.RowCount
	}
	return capacity
}

func visibleTableRowWindow(rowCount, selectedRow, offset, capacity int) (int, int) {
	if rowCount <= 0 || capacity <= 0 {
		return 0, 0
	}
	if capacity > rowCount {
		capacity = rowCount
	}
	if selectedRow < 0 {
		selectedRow = 0
	}
	if selectedRow >= rowCount {
		selectedRow = rowCount - 1
	}
	maxStart := rowCount - capacity
	if offset < 0 {
		offset = 0
	}
	if offset > maxStart {
		offset = maxStart
	}
	if selectedRow < offset {
		offset = selectedRow
	} else if selectedRow >= offset+capacity {
		offset = selectedRow - capacity + 1
	}
	if offset > maxStart {
		offset = maxStart
	}
	return offset, offset + capacity
}

func preferredTableWidths(cols []tableColumn) []int {
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = col.Width
		if widths[i] < tableMinColumnWidth {
			widths[i] = tableMinColumnWidth
		}
	}
	return widths
}

// minimumReadableTableWidths derives the tabular compression floor from the
// current table instead of from a terminal-width breakpoint. Preferred width
// remains the V1 upper target; the floor protects a conservative fraction of
// that target and any title/cell content that already fits inside it.
func minimumReadableTableWidths(cols []tableColumn, rows [][]string) []int {
	preferred := preferredTableWidths(cols)
	floors := make([]int, len(cols))
	for i, col := range cols {
		floor := ceilMulDiv(preferred[i], tableReadablePreferredNumerator, tableReadablePreferredDenominator)
		if floor < tableMinColumnWidth {
			floor = tableMinColumnWidth
		}

		observed := lipgloss.Width(SanitizeSingleLineText(col.Title))
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			if width := lipgloss.Width(SanitizeSingleLineText(row[i])); width > observed {
				observed = width
			}
		}
		if observed > preferred[i] {
			observed = preferred[i]
		}
		if observed > floor {
			floor = observed
		}
		if floor > preferred[i] {
			floor = preferred[i]
		}
		floors[i] = floor
	}
	return floors
}

func ceilMulDiv(value, numerator, denominator int) int {
	if value <= 0 || numerator <= 0 || denominator <= 0 {
		return 0
	}
	return (value*numerator + denominator - 1) / denominator
}

func tableColumnSeparatorWidth(variant string) int {
	if variant == "compact" || variant == "dense" {
		return tableCompactSeparatorW
	}
	return tableNormalSeparatorW
}

func masterDetailEligible(in TableLayoutInput, readable []int) bool {
	if !in.Selectable || in.RowCount <= 0 || len(in.Columns) < 4 || len(readable) == 0 {
		return false
	}
	masterMin, detailMin := masterDetailMinimumPaneWidths(in.Columns, readable)
	return in.AvailableWidth >= masterMin+tablePaneSeparatorWidth+detailMin
}

func masterDetailMinimumPaneWidths(cols []tableColumn, readable []int) (int, int) {
	masterMin := tableSelectablePrefixW + tableMinColumnWidth
	if len(readable) > 0 {
		masterMin = tableSelectablePrefixW + readable[0]
	}

	detailMin := tableMinColumnWidth
	for i, col := range cols {
		valueFloor := tableMinColumnWidth
		if i < len(readable) {
			valueFloor = readable[i]
		}
		lineWidth := lipgloss.Width(SanitizeSingleLineText(col.Title)) + 2 + valueFloor
		if lineWidth > detailMin {
			detailMin = lineWidth
		}
	}
	return masterMin, detailMin
}

func masterDetailPaneWidths(in TableLayoutInput, readable []int) (int, int) {
	masterMin, detailMin := masterDetailMinimumPaneWidths(in.Columns, readable)
	left := in.AvailableWidth * 2 / 5
	if left < masterMin {
		left = masterMin
	}
	maxLeft := in.AvailableWidth - tablePaneSeparatorWidth - detailMin
	if left > maxLeft {
		left = maxLeft
	}
	right := in.AvailableWidth - tablePaneSeparatorWidth - left
	return left, right
}

func masterColumnPrefix(widths []int, paneWidth, prefixW, separatorW int) []int {
	available := paneWidth - prefixW
	if available < tableMinColumnWidth || len(widths) == 0 {
		return nil
	}

	used := 0
	visible := make([]int, 0, len(widths))
	for i, width := range widths {
		if width < tableMinColumnWidth {
			width = tableMinColumnWidth
		}
		extra := width
		if len(visible) > 0 {
			extra += separatorW
		}
		if used+extra > available {
			break
		}
		visible = append(visible, i)
		used += extra
	}
	if len(visible) == 0 {
		visible = append(visible, 0)
	}
	return visible
}

func shrinkTableWidthsToFloors(widths, floors []int, budget int) []int {
	out := append([]int(nil), widths...)
	for sumTableWidths(out) > budget {
		widest := -1
		for i, width := range out {
			floor := tableMinColumnWidth
			if i < len(floors) && floors[i] > floor {
				floor = floors[i]
			}
			if width > floor && (widest < 0 || width > out[widest]) {
				widest = i
			}
		}
		if widest < 0 {
			break
		}
		out[widest]--
	}
	return out
}

// shrinkTableWidths is retained for internal callers that only need the base
// hard floor. Adaptive planning uses shrinkTableWidthsToFloors instead.
func shrinkTableWidths(widths []int, budget int) []int {
	floors := make([]int, len(widths))
	for i := range floors {
		floors[i] = tableMinColumnWidth
	}
	return shrinkTableWidthsToFloors(widths, floors, budget)
}

func allColumnIndexes(count int) []int {
	out := make([]int, count)
	for i := range out {
		out[i] = i
	}
	return out
}

func sumTableWidths(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width
	}
	return total
}
