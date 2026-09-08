package bubbletea

import (
	"encoding/json"

	"github.com/Ostrovsky42/agent-interaction-runtime/layout"
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the color palette used by the Bubble Tea adapter.
type Theme struct {
	Primary lipgloss.TerminalColor
	Success lipgloss.TerminalColor
	Warn    lipgloss.TerminalColor
	Error   lipgloss.TerminalColor
	Muted   lipgloss.TerminalColor
	Text    lipgloss.TerminalColor
	Border  lipgloss.TerminalColor
	Surface lipgloss.TerminalColor
	Focus   lipgloss.TerminalColor
}

// DefaultTheme provides a refined terminal dark palette.
var DefaultTheme = Theme{
	Primary: lipgloss.Color("36"),  // Cyan / Teal
	Success: lipgloss.Color("42"),  // Green
	Warn:    lipgloss.Color("214"), // Amber / Warm Orange
	Error:   lipgloss.Color("196"), // Red
	Muted:   lipgloss.Color("244"), // Gray
	Text:    lipgloss.Color("255"), // White
	Border:  lipgloss.Color("240"), // Subtle dark border
	Surface: lipgloss.Color("236"), // Subtle surface
	Focus:   lipgloss.Color("45"),  // Focus accent
}

// ColorFor maps protocol color tokens to lipgloss terminal colors.
func (t Theme) ColorFor(token string) lipgloss.TerminalColor {
	switch token {
	case "primary":
		return t.Primary
	case "warn":
		return t.Warn
	case "error":
		return t.Error
	case "muted":
		return t.Muted
	default:
		return t.Text
	}
}

// BorderFor maps protocol border types to lipgloss border definitions.
func BorderFor(b layout.Border) lipgloss.Border {
	switch b {
	case layout.BorderRounded:
		return lipgloss.RoundedBorder()
	case layout.BorderNormal:
		return lipgloss.NormalBorder()
	default:
		return lipgloss.HiddenBorder()
	}
}

// NodeStyle resolves protocol JSON style properties into a lipgloss Style.
func NodeStyle(raw json.RawMessage, theme Theme) lipgloss.Style {
	st := lipgloss.NewStyle()
	if len(raw) == 0 {
		return st
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return st
	}
	if fg, ok := m["fg"].(string); ok && fg != "" {
		st = st.Foreground(theme.ColorFor(fg))
	}
	if bg, ok := m["bg"].(string); ok && bg != "" {
		st = st.Background(theme.ColorFor(bg))
	}
	if bold, ok := m["bold"].(bool); ok && bold {
		st = st.Bold(true)
	}
	if dim, ok := m["dim"].(bool); ok && dim {
		st = st.Faint(true)
	}
	return st
}
