package bubbletea

import (
	"encoding/json"
	"fmt"
	"strings"

	"a2ui/document"
	"a2ui/layout"
	a2runtime "a2ui/runtime"
	"github.com/charmbracelet/lipgloss"
)

func (r *Renderer) renderText(n document.Node, maxW int) string {
	text := SanitizeText(propString(n, "text", "") + n.Text)
	variant := propString(n, "variant", "body")
	prof := profileFor(r.Preset)
	st := lipgloss.NewStyle().Foreground(r.Theme.Text)
	switch variant {
	case "title":
		text = prof.TitlePrefix + text
		st = st.Bold(true).Foreground(r.Theme.Primary)
	case "subtitle":
		text = prof.SubtitlePrefix + text
		st = st.Foreground(r.Theme.Muted)
	case "label":
		st = st.Bold(true)
	case "code":
		st = st.Foreground(r.Theme.Warn)
	case "muted":
		st = st.Foreground(r.Theme.Muted).Faint(true)
	}
	text = wrapPlainText(text, maxW)
	st = applyRawStyle(st, n.Props["style"], r.Theme)
	return st.Render(text)
}

func (r *Renderer) renderInput(n document.Node, focused bool, currentValue string, caret, maxW int, state RenderState) string {
	placeholder := SanitizeSingleLineText(propString(n, "placeholder", ""))
	currentValue = SanitizeSingleLineText(currentValue)
	border := lipgloss.RoundedBorder()
	if r.Preset == PresetDense {
		border = lipgloss.NormalBorder()
	}
	st := lipgloss.NewStyle().BorderStyle(border)
	if r.Preset != PresetDense {
		st = st.Padding(0, 1)
	}
	if focused {
		st = st.BorderForeground(r.Theme.Focus).Bold(true)
	} else {
		st = st.BorderForeground(r.Theme.Muted)
	}
	if maxW < 3 {
		return fitPlainText(currentValue, maxW)
	}
	contentW := maxW - 2
	if r.Preset != PresetDense {
		contentW -= 2
	}
	if contentW < 1 {
		contentW = 1
	}

	if !focused {
		display := currentValue
		if display == "" {
			display = placeholder
			st = st.Foreground(r.Theme.Muted).Faint(true)
		}
		return st.Render(fitPlainText(display, contentW))
	}

	display := inputWindowWithCaret(currentValue, caret, contentW, state.CursorVisible)
	return st.Render(display)
}

func inputWindowWithCaret(value string, caret, width int, cursorVisible bool) string {
	if width < 1 {
		return ""
	}
	runes := []rune(value)
	if caret < 0 {
		caret = 0
	}
	if caret > len(runes) {
		caret = len(runes)
	}
	cursor := " "
	if cursorVisible {
		cursor = "█"
	}
	cursorW := maxInt(lipgloss.Width(cursor), 1)
	if cursorW > width {
		return fitPlainText(cursor, width)
	}

	leftBudget := (width - cursorW) / 2
	start := caret
	usedLeft := 0
	for start > 0 {
		rw := maxInt(lipgloss.Width(string(runes[start-1])), 1)
		if usedLeft+rw > leftBudget {
			break
		}
		start--
		usedLeft += rw
	}

	end := caret
	used := usedLeft + cursorW
	for end < len(runes) {
		rw := maxInt(lipgloss.Width(string(runes[end])), 1)
		if used+rw > width {
			break
		}
		used += rw
		end++
	}
	for start > 0 {
		rw := maxInt(lipgloss.Width(string(runes[start-1])), 1)
		if used+rw > width {
			break
		}
		start--
		used += rw
	}
	return string(runes[start:caret]) + cursor + string(runes[caret:end])
}

func (r *Renderer) renderActions(n document.Node, maxW int) string {
	type item struct{ Key, Label string }
	var items []item
	_ = json.Unmarshal(n.Props["items"], &items)
	variant := propString(n, "variant", "inline")
	prof := profileFor(r.Preset)
	keyStyle := lipgloss.NewStyle().Foreground(r.Theme.Primary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(r.Theme.Text)
	muted := lipgloss.NewStyle().Foreground(r.Theme.Muted)
	parts := make([]string, 0, len(items))
	for _, it := range items {
		key := SanitizeSingleLineText(it.Key)
		label := SanitizeSingleLineText(it.Label)
		var p string
		switch variant {
		case "toolbar":
			if r.Preset == PresetDense {
				p = keyStyle.Render("["+key+"]") + " " + labelStyle.Render(label)
			} else {
				p = muted.Render("[") + keyStyle.Render(" "+key+" ") + labelStyle.Render(label) + muted.Render(" ]")
			}
		case "list":
			p = keyStyle.Render("["+key+"]") + " " + labelStyle.Render(label)
		default:
			p = muted.Render("[") + keyStyle.Render(key) + muted.Render(":") + labelStyle.Render(label) + muted.Render("]")
		}
		parts = append(parts, p)
	}
	if variant == "list" {
		return strings.Join(parts, "\n")
	}
	sep := strings.Repeat(" ", maxInt(prof.ActionGap, 1))
	return packInline(parts, sep, maxW)
}

func (r *Renderer) renderProgress(n document.Node, maxW int, state RenderState) string {
	var val float64
	_ = json.Unmarshal(n.Props["value"], &val)
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}
	label := SanitizeSingleLineText(propString(n, "label", ""))
	variant := propString(n, "variant", "bar")
	status := propString(n, "state", "normal")
	if status == "success" {
		return lipgloss.NewStyle().Foreground(r.Theme.Success).Bold(true).Render("✓ " + fitPlainText(label, maxInt(maxW-2, 1)))
	}
	if status == "error" {
		return lipgloss.NewStyle().Foreground(r.Theme.Error).Bold(true).Render("✕ " + fitPlainText(label, maxInt(maxW-2, 1)))
	}
	if variant == "spinner" {
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		glyph := "•"
		if status == "loading" {
			glyph = frames[state.AnimationPhase%uint64(len(frames))]
		}
		return lipgloss.NewStyle().Foreground(r.Theme.Primary).Render(glyph) + " " + fitPlainText(label, maxInt(maxW-2, 1))
	}
	pct := int(val * 100)
	if variant == "compact" || maxW < 12 {
		return fmt.Sprintf("%3d%% %s", pct, fitPlainText(label, maxInt(maxW-5, 1)))
	}
	prof := profileFor(r.Preset)
	pctText := fmt.Sprintf("%3d%%", pct)
	suffix := " " + pctText
	if label != "" {
		suffix += " " + label
	}
	barW := maxW - lipgloss.Width(suffix) - 2
	if barW < 3 {
		barW = 3
	}
	filled := int(float64(barW) * val)
	if status == "loading" && filled < barW {
		pulse := int(state.AnimationPhase % uint64(barW))
		if pulse >= filled {
			filled = pulse + 1
		}
	}
	if filled > barW {
		filled = barW
	}
	fill := lipgloss.NewStyle().Foreground(r.Theme.Primary).Render(strings.Repeat(prof.ProgressBarGlyph, filled))
	empty := lipgloss.NewStyle().Foreground(r.Theme.Muted).Render(strings.Repeat(prof.ProgressEmpty, barW-filled))
	return "[" + fill + empty + "]" + fitPlainText(suffix, maxInt(maxW-barW-2, 1))
}

type tableColumn struct {
	Title string `json:"title"`
	Width int    `json:"width"`
}

func (r *Renderer) renderTable(n document.Node, focused bool, selection a2runtime.TableSelection, maxW int) string {
	selectedRow := selection.Index
	var cols []tableColumn
	var rows [][]string
	_ = json.Unmarshal(n.Props["columns"], &cols)
	_ = json.Unmarshal(n.Props["rows"], &rows)
	for i := range cols {
		cols[i].Title = SanitizeSingleLineText(cols[i].Title)
	}
	for i := range rows {
		for j := range rows[i] {
			rows[i][j] = SanitizeSingleLineText(rows[i][j])
		}
	}
	selectable := propBool(n, "selectable", false)
	variant := propString(n, "variant", "normal")
	if len(cols) == 0 {
		return ""
	}
	if selectedRow >= len(rows) {
		selectedRow = len(rows) - 1
	}
	if selectedRow < 0 {
		selectedRow = 0
	}
	prefixW := 0
	if selectable {
		prefixW = 2
	}
	separator := " │ "
	if variant == "compact" || variant == "dense" {
		separator = " "
	}
	sepW := lipgloss.Width(separator) * maxInt(len(cols)-1, 0)
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = c.Width
		if widths[i] < 3 {
			widths[i] = 3
		}
	}
	minimum := prefixW + sepW + 3*len(cols)
	if maxW > 0 && minimum > maxW {
		return r.renderTableRecords(cols, rows, focused, selectable, selectedRow, maxW)
	}
	if maxW > 0 {
		for prefixW+sepW+sum(widths) > maxW {
			best := -1
			for i := range widths {
				if widths[i] > 3 && (best < 0 || widths[i] > widths[best]) {
					best = i
				}
			}
			if best < 0 {
				break
			}
			widths[best]--
		}
	}
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(r.Theme.Primary)
	dividerStyle := lipgloss.NewStyle().Foreground(r.Theme.Muted)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(r.Theme.Warn)
	var lines []string
	header := make([]string, len(cols))
	divider := make([]string, len(cols))
	for i, c := range cols {
		header[i] = padPlain(c.Title, widths[i])
		divider[i] = strings.Repeat("─", widths[i])
	}
	pre := ""
	if selectable {
		pre = "  "
	}
	lines = append(lines, pre+headerStyle.Render(strings.Join(header, separator)))
	if variant != "dense" {
		dividerSep := "─┼─"
		if variant == "compact" {
			dividerSep = "┼"
		}
		lines = append(lines, pre+dividerStyle.Render(strings.Join(divider, dividerSep)))
	}
	for i, row := range rows {
		cells := make([]string, len(cols))
		for j := range cols {
			v := ""
			if j < len(row) {
				v = row[j]
			}
			cells[j] = padPlain(v, widths[j])
		}
		content := strings.Join(cells, separator)
		rowPre := ""
		if selectable {
			rowPre = "  "
			if focused && i == selectedRow {
				rowPre = "> "
			}
		}
		if selectable && i == selectedRow {
			content = selectedStyle.Render(content)
		}
		lines = append(lines, rowPre+content)
	}
	return strings.Join(lines, "\n")
}

func (r *Renderer) renderTableRecords(cols []tableColumn, rows [][]string, focused, selectable bool, selected, maxW int) string {
	if maxW < 1 {
		maxW = 1
	}
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(r.Theme.Primary)
	selectedStyle := lipgloss.NewStyle().Foreground(r.Theme.Warn)
	blocks := make([]string, 0, len(rows))
	for i, row := range rows {
		lines := make([]string, 0, len(cols))
		for j, c := range cols {
			value := ""
			if j < len(row) {
				value = row[j]
			}
			prefix := c.Title + ": "
			valueW := maxInt(maxW-lipgloss.Width(prefix), 1)
			line := labelStyle.Render(fitPlainText(c.Title, maxInt(maxW-2, 1))) + ": " + fitPlainText(value, valueW)
			if selectable && focused && i == selected {
				line = selectedStyle.Render("> ") + line
			}
			lines = append(lines, line)
		}
		blocks = append(blocks, strings.Join(lines, "\n"))
	}
	return strings.Join(blocks, "\n\n")
}

func (r *Renderer) renderViewport(ctx renderContext, n document.Node, w, h int) string {
	vpH := layout.ClampViewportHeight(propInt(n, "height", 10), h)
	follow := propBool(n, "follow_tail", false)
	wrap := propBool(n, "wrap", true)
	parts := make([]string, 0, len(n.Children)+1)
	if n.Text != "" {
		sanitizedText := SanitizeText(n.Text)
		if wrap {
			parts = append(parts, wrapPlainText(sanitizedText, w))
		} else {
			logical := strings.Split(sanitizedText, "\n")
			for i := range logical {
				logical[i] = fitPlainText(logical[i], w)
			}
			parts = append(parts, strings.Join(logical, "\n"))
		}
	}
	for _, cid := range n.Children {
		v := r.renderNode(ctx, cid, w, 1<<20)
		if v != "" {
			parts = append(parts, v)
		}
	}
	content := strings.Join(parts, "\n")
	var lines []string
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	total := len(lines)
	visible := vpH
	if total < visible {
		visible = total
	}
	if visible < 0 {
		visible = 0
	}
	maxOffset := total - vpH
	if maxOffset < 0 {
		maxOffset = 0
	}
	local := ViewportState{PinnedToTail: follow}
	if ctx.interaction.Viewports != nil {
		if v, ok := ctx.interaction.Viewports[n.ID]; ok {
			local = v
		}
	}
	offset := local.Offset
	if follow && local.PinnedToTail {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	ctx.viewports[n.ID] = ViewportMetrics{TotalLines: total, VisibleLines: visible, MaxOffset: maxOffset, Offset: offset}
	if total == 0 || vpH == 0 {
		return ""
	}
	end := offset + vpH
	if end > total {
		end = total
	}
	return strings.Join(lines[offset:end], "\n")
}

func packInline(parts []string, sep string, maxW int) string {
	if len(parts) == 0 {
		return ""
	}
	if maxW <= 0 {
		return strings.Join(parts, sep)
	}
	var lines []string
	current := ""
	for _, part := range parts {
		candidate := part
		if current != "" {
			candidate = current + sep + part
		}
		if current != "" && lipgloss.Width(candidate) > maxW {
			lines = append(lines, current)
			current = part
		} else {
			current = candidate
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}
