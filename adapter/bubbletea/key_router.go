package bubbletea

import (
	"encoding/json"

	"github.com/Ostrovsky42/agent-interaction-runtime/document"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	a2runtime "github.com/Ostrovsky42/agent-interaction-runtime/runtime"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey implements deterministic routing:
// process control -> focus traversal -> focused component -> global actions.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.Quitting = true
		m.stopAnimation()
		return m, tea.Quit
	case tea.KeyTab:
		m.focusNext()
		m.reconcileLocalState()
		return m, m.animationCommandIfNeeded()
	case tea.KeyShiftTab:
		m.focusPrev()
		m.reconcileLocalState()
		return m, m.animationCommandIfNeeded()
	}

	snapshot := m.semanticSnapshot()
	if snapshot.FocusedID != "" {
		if n, ok := snapshot.Document.Nodes[snapshot.FocusedID]; ok {
			consumed := false
			switch n.Type {
			case protocol.NodeInput:
				consumed = m.handleInputKey(snapshot.FocusedID, snapshot.InputValues[snapshot.FocusedID], msg)
			case protocol.NodeTable:
				if propBool(n, "selectable", false) {
					consumed = m.handleTableKey(snapshot.FocusedID, n, msg)
				}
			case protocol.NodeViewport:
				if propBool(n, "scrollable", false) {
					consumed = m.handleViewportKey(snapshot.FocusedID, n, msg)
				}
			}
			if consumed {
				return m, m.animationCommandIfNeeded()
			}
		}
	}

	key := msg.String()
	if key != "" && m.controller != nil {
		if binding, ok := snapshot.Bindings[key]; ok {
			_ = m.controller.InvokeAction(binding.NodeID, binding.Action, binding.Args)
			return m, nil
		}
	}
	return m, nil
}

func (m *Model) handleInputKey(id, value string, msg tea.KeyMsg) bool {
	m.interaction = m.interaction.clone()
	runes := []rune(value)
	caret, ok := m.interaction.InputCarets[id]
	if !ok {
		caret = len(runes)
	}
	caret = clampInt(caret, 0, len(runes))

	switch msg.Type {
	case tea.KeyLeft:
		if caret > 0 {
			caret--
		}
	case tea.KeyRight:
		if caret < len(runes) {
			caret++
		}
	case tea.KeyHome:
		caret = 0
	case tea.KeyEnd:
		caret = len(runes)
	case tea.KeyBackspace:
		if caret > 0 {
			runes = append(runes[:caret-1], runes[caret:]...)
			caret--
			_ = m.controller.SetInput(id, string(runes))
		}
	case tea.KeyDelete:
		if caret < len(runes) {
			runes = append(runes[:caret], runes[caret+1:]...)
			_ = m.controller.SetInput(id, string(runes))
		}
	case tea.KeyRunes:
		if len(msg.Runes) > 0 {
			insert := append([]rune(nil), msg.Runes...)
			next := make([]rune, 0, len(runes)+len(insert))
			next = append(next, runes[:caret]...)
			next = append(next, insert...)
			next = append(next, runes[caret:]...)
			caret += len(insert)
			_ = m.controller.SetInput(id, string(next))
		}
	case tea.KeySpace:
		next := make([]rune, 0, len(runes)+1)
		next = append(next, runes[:caret]...)
		next = append(next, ' ')
		next = append(next, runes[caret:]...)
		caret++
		_ = m.controller.SetInput(id, string(next))
	case tea.KeyEnter:
		_ = m.controller.Submit(id)
	default:
		return false
	}
	m.interaction.InputCarets[id] = caret
	return true
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
	var delta int
	absolute := false
	target := 0

	metrics, ok := m.renderResult().Viewports[id]
	if !ok {
		return false
	}
	page := metrics.VisibleLines - 1
	if page < 1 {
		page = 1
	}
	switch msg.Type {
	case tea.KeyUp:
		delta = -1
	case tea.KeyDown:
		delta = 1
	case tea.KeyPgUp:
		delta = -page
	case tea.KeyPgDown:
		delta = page
	case tea.KeyHome:
		absolute = true
		target = 0
	case tea.KeyEnd:
		absolute = true
		target = metrics.MaxOffset
	default:
		return false
	}

	m.interaction = m.interaction.clone()
	local, exists := m.interaction.Viewports[id]
	if !exists {
		local = ViewportState{PinnedToTail: propBool(n, "follow_tail", false)}
	}
	if !absolute {
		target = metrics.Offset + delta
	}
	target = clampInt(target, 0, metrics.MaxOffset)
	follow := propBool(n, "follow_tail", false)
	local.Offset = target
	if !follow {
		local.PinnedToTail = false
	} else if msg.Type == tea.KeyEnd || ((msg.Type == tea.KeyDown || msg.Type == tea.KeyPgDown) && target == metrics.MaxOffset) {
		local.PinnedToTail = true
	} else if target < metrics.MaxOffset || msg.Type == tea.KeyHome || msg.Type == tea.KeyUp || msg.Type == tea.KeyPgUp {
		local.PinnedToTail = false
	}
	m.interaction.Viewports[id] = local
	return true
}

func (m Model) focusNext() {
	snapshot := m.semanticSnapshot()
	ids := a2runtime.FocusableIDs(snapshot.Document)
	if len(ids) == 0 {
		return
	}
	curr := snapshot.FocusedID
	idx := -1
	for i, id := range ids {
		if id == curr {
			idx = i
			break
		}
	}
	_ = m.controller.Focus(ids[(idx+1)%len(ids)])
}

func (m Model) focusPrev() {
	snapshot := m.semanticSnapshot()
	ids := a2runtime.FocusableIDs(snapshot.Document)
	if len(ids) == 0 {
		return
	}
	curr := snapshot.FocusedID
	idx := -1
	for i, id := range ids {
		if id == curr {
			idx = i
			break
		}
	}
	if idx <= 0 {
		idx = len(ids)
	}
	_ = m.controller.Focus(ids[idx-1])
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
