package bubbletea

import (
	"encoding/json"
	"sort"
	"strings"

	"a2ui/document"
	"a2ui/protocol"
	"github.com/charmbracelet/lipgloss"
)

type contextFooterAction struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// renderContextFooter derives terminal-local help from the focused component
// and the already-declared semantic actions. It never mutates Document/runtime
// state and never registers built-in navigation as semantic actions.
func (r *Renderer) renderContextFooter(doc document.Document, focusedID string, maxW int) string {
	focused, ok := doc.Nodes[focusedID]
	if !ok || maxW < 1 {
		return ""
	}

	var parts []string
	switch focused.Type {
	case protocol.NodeTable:
		if !propBool(focused, "selectable", false) {
			return ""
		}
		parts = append(parts, "↑↓ Move", "PgUp/PgDn Page", "Home/End Jump", "Enter Select", "Tab Focus")
	case protocol.NodeViewport:
		if !propBool(focused, "scrollable", false) {
			return ""
		}
		parts = append(parts, "↑↓ Scroll", "PgUp/PgDn Page", "Home/End Edge", "Tab Focus")
	case protocol.NodeInput:
		parts = append(parts, "←→ Cursor", "Enter Submit", "Tab Focus")
	default:
		return ""
	}

	// V1 action keys are single runes. A focused input owns rune entry before
	// global semantic actions, so advertising those action keys there would lie.
	// Special navigation names such as "down" are rejected by the frozen schema
	// and therefore cannot shadow table/viewport special-key commands.
	if focused.Type != protocol.NodeInput {
		parts = append(parts, contextFooterActions(doc)...)
	}

	line := strings.Join(parts, "   ")
	line = fitPlainText(line, maxW)
	return lipgloss.NewStyle().Foreground(r.Theme.Muted).Render(line)
}

func contextFooterActions(doc document.Document) []string {
	items := make([]contextFooterAction, 0)
	var walk func(string)
	walk = func(id string) {
		n, ok := doc.Nodes[id]
		if !ok {
			return
		}
		if n.Type == protocol.NodeActions {
			var nodeItems []contextFooterAction
			_ = json.Unmarshal(n.Props["items"], &nodeItems)
			items = append(items, nodeItems...)
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk("root")

	// Bindings are screen-global and duplicate keys are rejected by runtime.
	// Sort here so renderer chrome remains deterministic even if an agent changes
	// where action nodes live in the semantic tree.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Key == items[j].Key {
			return items[i].Label < items[j].Label
		}
		return items[i].Key < items[j].Key
	})

	out := make([]string, 0, len(items))
	for _, item := range items {
		key := strings.ToUpper(SanitizeSingleLineText(item.Key))
		label := SanitizeSingleLineText(item.Label)
		if key == "" || label == "" {
			continue
		}
		out = append(out, "["+key+"] "+label)
	}
	return out
}
