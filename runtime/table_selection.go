package runtime

import (
	"encoding/json"
	"fmt"

	"a2ui/document"
	"a2ui/protocol"
)

// TableSelection is renderer-independent semantic interaction state. RowID is
// populated when the table supplies stable row_ids; otherwise Index is the
// compatibility identity.
type TableSelection struct {
	Index int
	RowID string
}

func tableRows(n document.Node) [][]string {
	var rows [][]string
	if raw := n.Props["rows"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &rows)
	}
	return rows
}

func tableRowIDs(n document.Node) []string {
	var ids []string
	if raw := n.Props["row_ids"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &ids)
	}
	return ids
}

func tableSelectionAt(n document.Node, index int) (TableSelection, bool) {
	rows := tableRows(n)
	if len(rows) == 0 {
		return TableSelection{}, false
	}
	if index < 0 {
		index = 0
	}
	if index >= len(rows) {
		index = len(rows) - 1
	}
	ids := tableRowIDs(n)
	rowID := ""
	if len(ids) == len(rows) {
		rowID = ids[index]
	}
	return TableSelection{Index: index, RowID: rowID}, true
}

func reconcileSelection(n document.Node, current TableSelection, existed bool) (TableSelection, bool) {
	if n.Type != protocol.NodeTable || !propBool(n, "selectable") {
		return TableSelection{}, false
	}
	rows := tableRows(n)
	if len(rows) == 0 {
		return TableSelection{}, false
	}
	ids := tableRowIDs(n)
	if existed && current.RowID != "" && len(ids) == len(rows) {
		for i, id := range ids {
			if id == current.RowID {
				return TableSelection{Index: i, RowID: id}, true
			}
		}
	}
	index := 0
	if existed {
		index = current.Index
	}
	return tableSelectionAt(n, index)
}

func (s *State) reconcileTableSelections(next document.Document) {
	for id := range s.TableSelections {
		n, ok := next.Nodes[id]
		if !ok || n.Type != protocol.NodeTable || !propBool(n, "selectable") || len(tableRows(n)) == 0 {
			delete(s.TableSelections, id)
		}
	}
	for id, n := range next.Nodes {
		if n.Type != protocol.NodeTable || !propBool(n, "selectable") {
			continue
		}
		current, existed := s.TableSelections[id]
		if selection, ok := reconcileSelection(n, current, existed); ok {
			s.TableSelections[id] = selection
		} else {
			delete(s.TableSelections, id)
		}
	}
}

// TableSelection returns the current semantic selection for a selectable table.
func (s *State) TableSelection(id string) (TableSelection, bool) {
	sel, ok := s.TableSelections[id]
	return sel, ok
}

// SetTableSelection updates local semantic interaction state only. It requests
// a redraw but never advances protocol publication generation.
func (s *State) SetTableSelection(d document.Document, id string, index int) *protocol.Error {
	n, ok := d.Nodes[id]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", id))
	}
	if n.Type != protocol.NodeTable {
		return protocol.NewError("runtime.not_table", fmt.Sprintf("node %q is not table", id))
	}
	if !propBool(n, "selectable") {
		return protocol.NewError("runtime.not_selectable", fmt.Sprintf("table %q is not selectable", id))
	}
	next, has := tableSelectionAt(n, index)
	if !has {
		if _, existed := s.TableSelections[id]; existed {
			delete(s.TableSelections, id)
			s.RenderGeneration++
		}
		return nil
	}
	if current, existed := s.TableSelections[id]; existed && current == next {
		return nil
	}
	s.TableSelections[id] = next
	s.RenderGeneration++
	return nil
}

// MoveTableSelection moves the selected row without wrapping.
func (s *State) MoveTableSelection(d document.Document, id string, delta int) *protocol.Error {
	n, ok := d.Nodes[id]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", id))
	}
	if n.Type != protocol.NodeTable {
		return protocol.NewError("runtime.not_table", fmt.Sprintf("node %q is not table", id))
	}
	if !propBool(n, "selectable") {
		return protocol.NewError("runtime.not_selectable", fmt.Sprintf("table %q is not selectable", id))
	}
	rows := tableRows(n)
	if len(rows) == 0 {
		return nil
	}
	current := 0
	if sel, exists := s.TableSelections[id]; exists {
		current = sel.Index
	}
	return s.SetTableSelection(d, id, current+delta)
}

func (s *State) tableSelectionsSnapshot() map[string]TableSelection {
	out := make(map[string]TableSelection, len(s.TableSelections))
	for id, selection := range s.TableSelections {
		out[id] = selection
	}
	return out
}
