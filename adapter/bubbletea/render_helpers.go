package bubbletea

import (
	"encoding/json"
	"strings"

	"a2ui/document"
	"github.com/charmbracelet/lipgloss"
)

func propString(n document.Node, key, fallback string) string {
	var v string
	if raw := n.Props[key]; len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
		return SanitizeText(v)
	}
	return SanitizeText(fallback)
}
func propInt(n document.Node, key string, fallback int) int {
	var v int
	if raw := n.Props[key]; len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
		return v
	}
	return fallback
}
func propBool(n document.Node, key string, fallback bool) bool {
	var v bool
	if raw := n.Props[key]; len(raw) > 0 && json.Unmarshal(raw, &v) == nil {
		return v
	}
	return fallback
}

func applyRawStyle(st lipgloss.Style, raw json.RawMessage, theme Theme) lipgloss.Style {
	if len(raw) == 0 {
		return st
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return st
	}
	if fg, ok := m["fg"].(string); ok && fg != "" {
		st = st.Foreground(theme.ColorFor(fg))
	}
	if bg, ok := m["bg"].(string); ok && bg != "" {
		st = st.Background(theme.ColorFor(bg))
	}
	if b, ok := m["bold"].(bool); ok {
		st = st.Bold(b)
	}
	if d, ok := m["dim"].(bool); ok {
		st = st.Faint(d)
	}
	return st
}

func wrapPlainText(s string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	var out []string
	for _, logical := range strings.Split(s, "\n") {
		if logical == "" {
			out = append(out, "")
			continue
		}
		var line strings.Builder
		width := 0
		for _, r := range logical {
			rw := lipgloss.Width(string(r))
			if rw < 1 {
				rw = 1
			}
			if width > 0 && width+rw > maxW {
				out = append(out, line.String())
				line.Reset()
				width = 0
			}
			line.WriteRune(r)
			width += rw
		}
		out = append(out, line.String())
	}
	return strings.Join(out, "\n")
}

func fitPlainText(s string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if rw < 1 {
			rw = 1
		}
		if w+rw > maxW {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	return b.String()
}

func padPlain(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return fitPlainText(s, width)
	}
	return s + strings.Repeat(" ", width-w)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
