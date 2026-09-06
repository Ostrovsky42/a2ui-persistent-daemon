package bubbletea

import (
	"encoding/json"

	"a2ui/document"
	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
		m.Quitting = true
		return m, tea.Quit
	}

	snapshot := m.semanticSnapshot()
	focusedID := snapshot.FocusedID
	n, ok := snapshot.Document.Nodes[focusedID]
	if !ok {
		return m, nil
	}

	handled := false
	switch n.Type {
	case protocol.NodeInput:
		handled = m.handleInputKey(focusedID, snapshot.InputValues[focusedID], msg)
	case protocol.NodeTable:
		handled = m.handleTableKey(focusedID, n, msg)
	case protocol.NodeViewport:
		handled = m.handleViewportKey(focusedID, n, msg)
	}
	if handled {
		m.animationCommandIfNeeded()
		return m, nil
	}

	if msg.Type == tea.KeyTab {
		_ = m.controller.FocusNext(1)
		m.reconcileLocalState()
		return m, m.animationCommandIfNeeded()
	}
	if msg.Type == tea.KeyShiftTab {
		_ = m.controller.FocusNext(-1)
		m.reconcileLocalState()
		return m, m.animationCommandIfNeeded()
	}
	return m, nil
}

func (m *Model) handleInputKey(id, value string, msg tea.KeyMsg) bool {
	runes := []rune(value)
	caret := len(runes)
	if local, ok := m.interaction.InputCarets[id]; ok {
		caret = local
	}
	if caret < 0 {
		caret = 0
	}
	if caret > len(runes) {
		caret = len(runes)
	}

	switch msg.Type {
	case tea.KeyLeft:
		if caret > 0 {
			caret--
		}
		m.interaction.InputCarets[id] = caret
		return true
	case tea.KeyRight:
		if caret < len(runes) {
			caret++
		}
		m.interaction.InputCarets[id] = caret
		return true
	case tea.KeyHome:
		m.interaction.InputCarets[id] = 0
		return true
	case tea.KeyEnd:
		m.interaction.InputCarets[id] = len(runes)
		return true
	case tea.KeyBackspace:
		if caret == 0 {
			return true
		}
		next := append(append([]rune(nil), runes[:caret-1]...), runes[caret:]...)
		if err := m.controller.SetInputValue(id, string(next)); err == nil {
			m.interaction.InputCarets[id] = caret - 1
		}
		return true
	case tea.KeyDelete:
		if caret >= len(runes) {
			return true
		}
		next := append(append([]rune(nil), runes[:caret]...), runes[caret+1:]...)
		if err := m.controller.SetInputValue(id, string(next)); err == nil {
			m.interaction.InputCarets[id] = caret
		}
		return true
	case tea.KeyEnter:
		_ = m.controller.SubmitInput(id)
		return true
	case tea.KeyRunes:
		if len(msg.Runes) == 0 {
			return true
		}
		next := make([]rune, 0, len(runes)+len(msg.Runes))
		next = append(next, runes[:caret]...)
		next = append(next, msg.Runes...)
		next = append(next, runes[caret:]...)
		if err := m.controller.SetInputValue(id, string(next)); err == nil {
			m.interaction.InputCarets[id] = caret + len(msg.Runes)
		}
		return true
	}
	return false
}

func (m *Model) handleTableKey(id string, n document.Node, msg tea.KeyMsg) bool {
	selectionChanged := false
	switch msg.Type {
	case tea.KeyUp:
		_ = m.controller.MoveTableSelection(id, -1)
		selectionChanged = true
	case tea.KeyDown:
		_ = m.controller.MoveTableSelection(id, 1)
		selectionChanged = true
	case tea.KeyHome:
		snapshot := m.semanticSnapshot()
		if selection, ok := snapshot.TableSelections[id]; ok {
			_ = m.controller.MoveTableSelection(id, -selection.Index)
			selectionChanged = true
		}
	case tea.KeyEnd:
		var rows [][]string
		_ = json.Unmarshal(n.Props["rows"], &rows)
		if len(rows) > 0 {
			snapshot := m.semanticSnapshot()
			current := 0
			if selection, ok := snapshot.TableSelections[id]; ok {
				current = selection.Index
			}
			_ = m.controller.MoveTableSelection(id, len(rows)-1-current)
			selectionChanged = true
		}
	case tea.KeyEnter:
		_ = m.controller.ActivateTableSelection(id)
	default:
		return false
	}
	if selectionChanged {
		// Runtime owns semantic selection. Reconciliation combines that selection,
		// current geometry and existing adapter-local continuity state to resolve
		// the next visible row window.
		m.reconcileLocalState()
	}
	return true
}

func (m *Model) handleViewportKey(id string, n document.Node, msg tea.KeyMsg) bool {
	if !propBool(n, "scrollable", false) {
		return false
	}
	viewport := m.interaction.Viewports[id]
	metrics := m.renderResult().Viewports[id]
	page := metrics.VisibleLines
	if page < 1 {
		page = maxInt(m.Height-1, 1)
	}

	switch msg.Type {
	case tea.KeyUp:
		viewport.Offset--
		viewport.PinnedToTail = false
	case tea.KeyDown:
		viewport.Offset++
		viewport.PinnedToTail = false
	case tea.KeyPgUp:
		viewport.Offset -= page
		viewport.PinnedToTail = false
	case tea.KeyPgDown:
		viewport.Offset += page
		viewport.PinnedToTail = false
	case tea.KeyHome:
		viewport.Offset = 0
		viewport.PinnedToTail = false
	case tea.KeyEnd:
		viewport.Offset = metrics.MaxOffset
		viewport.PinnedToTail = true
	default:
		return false
	}
	if viewport.Offset < 0 {
		viewport.Offset = 0
	}
	if viewport.Offset > metrics.MaxOffset {
		viewport.Offset = metrics.MaxOffset
	}
	m.interaction.Viewports[id] = viewport
	return true
}
