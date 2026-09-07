package bubbletea

import (
	"encoding/json"
	"strings"

	"a2ui/document"
	"a2ui/layout"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
	"github.com/charmbracelet/lipgloss"
)

// Renderer maps immutable A2UI document/runtime snapshots to terminal frames.
// Preset and RenderState are presentation-only inputs and never mutate A2UI state.
type Renderer struct {
	Theme  Theme
	Preset Preset
}

func NewRenderer(theme Theme) *Renderer {
	return NewRendererWithPreset(theme, PresetMinimal)
}

func NewRendererWithPreset(theme Theme, preset Preset) *Renderer {
	if !preset.Valid() {
		preset = PresetMinimal
	}
	return &Renderer{Theme: theme, Preset: preset}
}

func (r *Renderer) RenderTree(doc document.Document, focusedID string, inputs map[string]string, tableSelected map[string]int, availW, availH int) string {
	return r.RenderTreeWithState(doc, focusedID, inputs, tableSelected, availW, availH, RenderState{CursorVisible: true})
}

// RenderTreeWithState preserves the V2 compatibility API. New code should use
// RenderFrame so table selection and adapter-local interaction state remain
// explicitly separated.
func (r *Renderer) RenderTreeWithState(doc document.Document, focusedID string, inputs map[string]string, tableSelected map[string]int, availW, availH int, state RenderState) string {
	selections := make(map[string]a2runtime.TableSelection, len(tableSelected))
	for id, index := range tableSelected {
		selections[id] = a2runtime.TableSelection{Index: index}
	}
	return r.RenderFrame(doc, focusedID, inputs, selections, InteractionState{}, availW, availH, state).Frame
}

// RenderFrame projects one immutable semantic snapshot plus adapter-local
// interaction mechanics into a terminal frame. Renderer itself retains no
// mutable session state.
func (r *Renderer) RenderFrame(doc document.Document, focusedID string, inputs map[string]string, tableSelections map[string]a2runtime.TableSelection, interaction InteractionState, availW, availH int, state RenderState) RenderResult {
	result := RenderResult{
		Viewports: make(map[string]ViewportMetrics),
		Tables:    make(map[string]TableViewportMetrics),
	}
	if _, ok := doc.Nodes["root"]; !ok {
		return result
	}
	if availW < 1 {
		availW = 1
	}
	if availH < 1 {
		availH = 1
	}

	footer := r.renderContextFooter(doc, focusedID, availW)
	contentH := availH
	if footer != "" {
		if availH > 1 {
			contentH--
		} else {
			// Never hide the entire semantic surface merely to show chrome.
			footer = ""
		}
	}

	ctx := renderContext{
		doc:             doc,
		focusedID:       focusedID,
		inputs:          inputs,
		tableSelections: tableSelections,
		interaction:     interaction,
		state:           state,
		viewports:       result.Viewports,
		tables:          result.Tables,
	}
	result.Frame = r.renderNode(ctx, "root", availW, contentH)
	if footer != "" {
		if result.Frame != "" {
			result.Frame += "\n"
		}
		result.Frame += footer
	}
	return result
}

type renderContext struct {
	doc             document.Document
	focusedID       string
	inputs          map[string]string
	tableSelections map[string]a2runtime.TableSelection
	interaction     InteractionState
	state           RenderState
	viewports       map[string]ViewportMetrics
	tables          map[string]TableViewportMetrics
}

func (r *Renderer) renderNode(ctx renderContext, id string, w, h int) string {
	n, ok := ctx.doc.Nodes[id]
	if !ok || w <= 0 || h <= 0 {
		return ""
	}
	switch n.Type {
	case protocol.NodeBox:
		return r.renderBox(ctx, n, w, h)
	case protocol.NodeText:
		return r.renderText(n, w)
	case protocol.NodeInput:
		caret := len([]rune(ctx.inputs[id]))
		if ctx.interaction.InputCarets != nil {
			if local, ok := ctx.interaction.InputCarets[id]; ok {
				caret = local
			}
		}
		return r.renderInput(n, ctx.focusedID == id, ctx.inputs[id], caret, w, ctx.state)
	case protocol.NodeActions:
		return r.renderActions(n, w)
	case protocol.NodeProgress:
		return r.renderProgress(n, w, ctx.state)
	case protocol.NodeTable:
		selection, ok := ctx.tableSelections[id]
		if !ok {
			selection = a2runtime.TableSelection{Index: 0}
		}
		rowOffset := 0
		if ctx.interaction.TableViewports != nil {
			if local, ok := ctx.interaction.TableViewports[id]; ok {
				rowOffset = local.Offset
			}
		}
		ctx.tables[id] = resolveTableViewportMetrics(n, selection, w, h, rowOffset)
		return r.renderTableWindowed(n, ctx.focusedID == id, selection, w, h, rowOffset)
	case protocol.NodeViewport:
		return r.renderViewport(ctx, n, w, h)
	default:
		return ""
	}
}

func (r *Renderer) renderBox(ctx renderContext, n document.Node, w, h int) string {
	dir := layout.Column
	if propString(n, "dir", "col") == "row" {
		dir = layout.Row
	}
	variant := propString(n, "variant", "plain")
	responsive := propString(n, "responsive", "none")
	align := propString(n, "align", "start")
	padding := propInt(n, "padding", 0)
	gap := propInt(n, "gap", 0)
	border := layout.Border(propString(n, "border", "none"))

	prof := profileFor(r.Preset)
	if variant != "plain" {
		if gap == 0 && !n.ExplicitProps["gap"] {
			gap = prof.DefaultGap
		}
		if padding == 0 && !n.ExplicitProps["padding"] {
			switch variant {
			case "card":
				padding = prof.CardPadding
			case "panel":
				padding = prof.PanelPadding
			case "section":
				padding = prof.SectionPadding
			}
		}
		if border == layout.BorderNone && !n.ExplicitProps["border"] {
			switch variant {
			case "card":
				border = prof.CardBorder
			case "panel":
				border = prof.PanelBorder
			case "section":
				border = prof.SectionBorder
			}
		}
	}

	metrics := layout.ComputeBox(w, h, layout.Box{Dir: dir, Padding: padding, Gap: gap, Border: border})
	childW, childH := metrics.ContentWidth, metrics.ContentHeight

	if dir == layout.Row && len(n.Children) > 0 {
		constraints := make([]layout.FlexConstraint, 0, len(n.Children))
		for _, cid := range n.Children {
			constraints = append(constraints, flexConstraint(ctx.doc.Nodes[cid]))
		}
		widths, fits := layout.AllocateRow(childW, gap, constraints)
		if !fits && responsive == "stack" {
			dir = layout.Column
		} else {
			if !fits {
				fallback := make([]layout.FlexConstraint, len(constraints))
				for i := range fallback {
					fallback[i] = layout.FlexConstraint{Grow: 1, Basis: 1, MinWidth: 1}
				}
				widths, _ = layout.AllocateRow(childW, gap, fallback)
			}
			views := make([]string, 0, len(n.Children))
			for i, cid := range n.Children {
				cw := 1
				if i < len(widths) && widths[i] > 0 {
					cw = widths[i]
				}
				views = append(views, r.renderNode(ctx, cid, cw, childH))
			}
			inner := joinHorizontal(align, gap, views)
			return r.decorateBox(n, variant, padding, border, inner)
		}
	}

	views := make([]string, 0, len(n.Children))
	remaining := childH
	for _, cid := range n.Children {
		if remaining < 1 {
			break
		}
		view := r.renderNode(ctx, cid, childW, remaining)
		if view == "" {
			continue
		}
		views = append(views, view)
		used := lipgloss.Height(view)
		if used < 1 {
			used = 1
		}
		remaining -= used
		if remaining > 0 && gap > 0 {
			remaining -= gap
		}
	}
	inner := joinVertical(align, gap, views)
	return r.decorateBox(n, variant, padding, border, inner)
}

func (r *Renderer) decorateBox(n document.Node, variant string, padding int, border layout.Border, inner string) string {
	rawStyle := n.Props["style"]
	if padding == 0 && (border == layout.BorderNone || border == "") && !n.ExplicitProps["style"] && !(variant == "panel" && r.Preset == PresetDashboard) {
		return inner
	}

	st := lipgloss.NewStyle().Padding(padding)
	if border != layout.BorderNone && border != "" {
		borderColor := r.Theme.Border
		if variant == "card" && r.Preset == PresetDashboard {
			borderColor = r.Theme.Primary
		}
		st = st.BorderStyle(BorderFor(border)).BorderForeground(borderColor)
	}
	if variant == "panel" && r.Preset == PresetDashboard {
		st = st.Foreground(r.Theme.Text)
	}
	st = applyRawStyle(st, rawStyle, r.Theme)
	return st.Render(inner)
}

func joinHorizontal(align string, gap int, views []string) string {
	if len(views) == 0 {
		return ""
	}
	items := make([]string, 0, len(views)*2-1)
	sep := strings.Repeat(" ", maxInt(gap, 0))
	for i, v := range views {
		if i > 0 && sep != "" {
			items = append(items, sep)
		}
		items = append(items, v)
	}
	pos := lipgloss.Top
	switch align {
	case "center":
		pos = lipgloss.Center
	case "end":
		pos = lipgloss.Bottom
	}
	return lipgloss.JoinHorizontal(pos, items...)
}

func joinVertical(align string, gap int, views []string) string {
	if len(views) == 0 {
		return ""
	}
	if align == "stretch" {
		maxW := 0
		for _, view := range views {
			maxW = maxInt(maxW, lipgloss.Width(view))
		}
		for i, view := range views {
			lines := strings.Split(view, "\n")
			for j, line := range lines {
				if width := lipgloss.Width(line); width < maxW {
					lines[j] = line + strings.Repeat(" ", maxW-width)
				}
			}
			views[i] = strings.Join(lines, "\n")
		}
	}
	gap = maxInt(gap, 0)
	items := make([]string, 0, len(views)+(len(views)-1)*gap)
	for i, v := range views {
		if i > 0 {
			for j := 0; j < gap; j++ {
				items = append(items, "")
			}
		}
		items = append(items, v)
	}
	pos := lipgloss.Left
	switch align {
	case "center":
		pos = lipgloss.Center
	case "end":
		pos = lipgloss.Right
	}
	joined := lipgloss.JoinVertical(pos, items...)
	if gap == 0 {
		return joined
	}

	// Lip Gloss pads zero-width separator items to the joined block width.
	// Keep alignment for real child content, but restore synthetic gap rows to
	// truly empty lines so gap=N has exactly N blank rows and no frame-only
	// trailing whitespace.
	lines := strings.Split(joined, "\n")
	line := 0
	for i, view := range views {
		line += lipgloss.Height(view)
		if i == len(views)-1 {
			break
		}
		for j := 0; j < gap && line < len(lines); j++ {
			lines[line] = ""
			line++
		}
	}
	return strings.Join(lines, "\n")
}

func flexConstraint(n document.Node) layout.FlexConstraint {
	var raw struct {
		Grow     int `json:"grow"`
		Basis    int `json:"basis"`
		MinWidth int `json:"min_width"`
		MaxWidth int `json:"max_width"`
	}
	b := n.Props["flex"]
	if len(b) == 0 {
		// Legacy row semantics shared the available width equally. Preserve that
		// behavior for documents that do not opt into explicit flex hints.
		return layout.FlexConstraint{Grow: 1, Basis: 1, MinWidth: 1}
	}
	_ = json.Unmarshal(b, &raw)
	return layout.FlexConstraint{Grow: raw.Grow, Basis: raw.Basis, MinWidth: raw.MinWidth, MaxWidth: raw.MaxWidth}
}

func (r *Renderer) HasActiveAnimation(doc document.Document, focusedID string) bool {
	if focusedID != "" {
		if n, ok := doc.Nodes[focusedID]; ok && n.Type == protocol.NodeInput {
			return true
		}
	}
	for _, n := range doc.Nodes {
		if n.Type != protocol.NodeProgress {
			continue
		}
		if propString(n, "state", "normal") == "loading" {
			v := propString(n, "variant", "bar")
			if v == "spinner" || v == "bar" {
				return true
			}
		}
	}
	return false
}
