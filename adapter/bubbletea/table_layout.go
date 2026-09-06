package bubbletea

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
	tableMinColumnWidth     = 3
	tableSelectablePrefixW  = 2
	tableNormalSeparatorW   = 3
	tableCompactSeparatorW  = 1
	tableMasterMinWidth     = 18
	tableDetailMinWidth     = 24
	tablePaneSeparatorWidth = 3
)

// TableLayoutInput contains only deterministic renderer inputs. Semantic
// selection remains runtime-owned; SelectedRow is read-only input here.
type TableLayoutInput struct {
	AvailableWidth  int
	AvailableHeight int
	Columns         []tableColumn
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
	VisibleCols    []int
	LeftPaneWidth  int
	RightPaneWidth int
	RowStart       int
	RowEnd         int
}

func planTableLayout(in TableLayoutInput) TableLayoutPlan {
	widths := preferredTableWidths(in.Columns)
	plan := TableLayoutPlan{
		Mode:         TableModeFull,
		ColumnWidths: append([]int(nil), widths...),
		VisibleCols:  allColumnIndexes(len(in.Columns)),
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

	if masterDetailEligible(in) {
		left, right := masterDetailPaneWidths(in.AvailableWidth)
		plan.Mode = TableModeMasterDetail
		plan.LeftPaneWidth = left
		plan.RightPaneWidth = right
		plan.VisibleCols = masterColumnPrefix(widths, left, prefixW, separatorW)
		return finishTablePlan(in, plan)
	}

	minimumTotal := prefixW + separatorTotal + tableMinColumnWidth*len(in.Columns)
	if minimumTotal <= in.AvailableWidth {
		plan.Mode = TableModeCompressed
		plan.ColumnWidths = shrinkTableWidths(widths, in.AvailableWidth-prefixW-separatorTotal)
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

func tableColumnSeparatorWidth(variant string) int {
	if variant == "compact" || variant == "dense" {
		return tableCompactSeparatorW
	}
	return tableNormalSeparatorW
}

func masterDetailEligible(in TableLayoutInput) bool {
	if !in.Selectable || in.RowCount <= 0 || len(in.Columns) < 4 {
		return false
	}
	minimum := tableMasterMinWidth + tablePaneSeparatorWidth + tableDetailMinWidth
	return in.AvailableWidth >= minimum
}

func masterDetailPaneWidths(available int) (int, int) {
	left := available * 2 / 5
	if left < tableMasterMinWidth {
		left = tableMasterMinWidth
	}
	maxLeft := available - tablePaneSeparatorWidth - tableDetailMinWidth
	if left > maxLeft {
		left = maxLeft
	}
	right := available - tablePaneSeparatorWidth - left
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

func shrinkTableWidths(widths []int, budget int) []int {
	out := append([]int(nil), widths...)
	for sumTableWidths(out) > budget {
		widest := -1
		for i, width := range out {
			if width > tableMinColumnWidth && (widest < 0 || width > out[widest]) {
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
