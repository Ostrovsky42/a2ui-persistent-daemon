package bubbletea

import (
	"encoding/json"

	"a2ui/document"
	a2runtime "a2ui/runtime"
)

// renderTableWindowed applies renderer-local vertical windowing before handing
// the projected row slice to the existing adaptive table renderer. The source
// node and runtime selection are copied; authoritative state is never mutated.
func (r *Renderer) renderTableWindowed(n document.Node, focused bool, selection a2runtime.TableSelection, maxW, maxH, rowOffset int) string {
	var cols []tableColumn
	var rows [][]string
	_ = json.Unmarshal(n.Props["columns"], &cols)
	_ = json.Unmarshal(n.Props["rows"], &rows)
	if len(cols) == 0 || len(rows) == 0 {
		return r.renderTable(n, focused, selection, maxW)
	}

	selectedRow := selection.Index
	if selectedRow < 0 {
		selectedRow = 0
	}
	if selectedRow >= len(rows) {
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
	if plan.RowStart == 0 && plan.RowEnd == len(rows) {
		return r.renderTable(n, focused, selection, maxW)
	}

	projected := n
	projected.Props = cloneNodeProps(n.Props)
	projectedRows := append([][]string(nil), rows[plan.RowStart:plan.RowEnd]...)
	rowsJSON, _ := json.Marshal(projectedRows)
	projected.Props["rows"] = rowsJSON

	if rawIDs := n.Props["row_ids"]; len(rawIDs) > 0 {
		var rowIDs []string
		if json.Unmarshal(rawIDs, &rowIDs) == nil && len(rowIDs) == len(rows) {
			idsJSON, _ := json.Marshal(append([]string(nil), rowIDs[plan.RowStart:plan.RowEnd]...))
			projected.Props["row_ids"] = idsJSON
		}
	}

	projectedSelection := selection
	projectedSelection.Index = selectedRow - plan.RowStart
	return r.renderTable(projected, focused, projectedSelection, maxW)
}

func cloneNodeProps(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for key, value := range in {
		out[key] = append(json.RawMessage(nil), value...)
	}
	return out
}
