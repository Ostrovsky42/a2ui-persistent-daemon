package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"a2ui/document"
	"a2ui/layout"
	"a2ui/protocol"
)

// Cell represents a single terminal cell.
type Cell struct {
	R    rune
	Fg   string
	Bg   string
	Bold bool
	Dim  bool
}

// Frame is a rendered 2D terminal screen.
type Frame struct {
	Width  int
	Height int
	Cells  [][]Cell
}

func newFrame(w, h int) *Frame {
	if w < 1 {
		w = 80
	}
	if h < 1 {
		h = 24
	}
	cells := make([][]Cell, h)
	for y := 0; y < h; y++ {
		cells[y] = make([]Cell, w)
		for x := 0; x < w; x++ {
			cells[y][x] = Cell{R: ' '}
		}
	}
	return &Frame{Width: w, Height: h, Cells: cells}
}

func (f *Frame) Set(x, y int, r rune, fg string, bold, dim bool) {
	if x >= 0 && x < f.Width && y >= 0 && y < f.Height {
		f.Cells[y][x] = Cell{R: r, Fg: fg, Bold: bold, Dim: dim}
	}
}

func (f *Frame) DrawString(x, y int, s string, fg string, bold, dim bool) {
	col := x
	for _, r := range s {
		if col >= f.Width {
			break
		}
		if r == '\n' {
			y++
			col = x
			continue
		}
		f.Set(col, y, r, fg, bold, dim)
		col++
	}
}

// PlainText returns the screen buffer without any ANSI codes.
func (f *Frame) PlainText() string {
	var sb strings.Builder
	for y := 0; y < f.Height; y++ {
		line := make([]rune, f.Width)
		for x := 0; x < f.Width; x++ {
			line[x] = f.Cells[y][x].R
		}
		trimmed := strings.TrimRight(string(line), " ")
		sb.WriteString(trimmed)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// ANSI returns the screen buffer formatted with ANSI escape codes.
func (f *Frame) ANSI() string {
	var sb strings.Builder
	colorCode := func(c string) string {
		switch c {
		case "primary":
			return "\033[36m" // cyan
		case "warn":
			return "\033[33m" // yellow
		case "error":
			return "\033[31m" // red
		case "muted":
			return "\033[90m" // dark gray
		case "success":
			return "\033[32m" // green
		default:
			return ""
		}
	}

	for y := 0; y < f.Height; y++ {
		lastFg := ""
		lastBold := false
		lastDim := false

		for x := 0; x < f.Width; x++ {
			cell := f.Cells[y][x]
			changed := cell.Fg != lastFg || cell.Bold != lastBold || cell.Dim != lastDim
			if changed {
				sb.WriteString("\033[0m")
				if cell.Bold {
					sb.WriteString("\033[1m")
				}
				if cell.Dim {
					sb.WriteString("\033[2m")
				}
				if c := colorCode(cell.Fg); c != "" {
					sb.WriteString(c)
				}
				lastFg = cell.Fg
				lastBold = cell.Bold
				lastDim = cell.Dim
			}
			sb.WriteRune(cell.R)
		}
		sb.WriteString("\033[0m\n")
	}
	return sb.String()
}

// Lines returns the frame split into plain text lines.
func (f *Frame) Lines() []string {
	return strings.Split(f.PlainText(), "\n")
}

// TerminalRenderer renders Document and State into a Frame.
type TerminalRenderer struct {
	Width  int
	Height int
}

func NewTerminalRenderer(w, h int) *TerminalRenderer {
	if w < 10 {
		w = 80
	}
	if h < 5 {
		h = 24
	}
	return &TerminalRenderer{Width: w, Height: h}
}

type rect struct {
	x, y, w, h int
}

func (tr *TerminalRenderer) Render(doc document.Document, focusedID string, inputValues map[string]string) *Frame {
	f := newFrame(tr.Width, tr.Height)
	if _, ok := doc.Nodes["root"]; !ok {
		return f
	}
	tr.renderNode(f, doc, "root", rect{x: 0, y: 0, w: tr.Width, h: tr.Height}, focusedID, inputValues)
	return f
}

func (tr *TerminalRenderer) renderNode(f *Frame, doc document.Document, id string, r rect, focusedID string, inputs map[string]string) {
	n, ok := doc.Nodes[id]
	if !ok || r.w <= 0 || r.h <= 0 {
		return
	}

	switch n.Type {
	case protocol.NodeBox:
		tr.renderBox(f, doc, n, r, focusedID, inputs)
	case protocol.NodeText:
		tr.renderText(f, n, r)
	case protocol.NodeInput:
		tr.renderInput(f, n, r, focusedID == id, inputs[id])
	case protocol.NodeActions:
		tr.renderActions(f, n, r)
	case protocol.NodeProgress:
		tr.renderProgress(f, n, r)
	case protocol.NodeTable:
		tr.renderTable(f, n, r)
	case protocol.NodeViewport:
		tr.renderViewport(f, doc, n, r, focusedID, inputs)
	}
}

func (tr *TerminalRenderer) renderBox(f *Frame, doc document.Document, n document.Node, r rect, focusedID string, inputs map[string]string) {
	var dir layout.Direction = layout.Column
	var border layout.Border = layout.BorderNone
	padding := 0
	gap := 0

	if raw, ok := n.Props["dir"]; ok {
		var d string
		_ = json.Unmarshal(raw, &d)
		if d == "row" {
			dir = layout.Row
		}
	}
	if raw, ok := n.Props["border"]; ok {
		var b string
		_ = json.Unmarshal(raw, &b)
		border = layout.Border(b)
	}
	if raw, ok := n.Props["padding"]; ok {
		_ = json.Unmarshal(raw, &padding)
	}
	if raw, ok := n.Props["gap"]; ok {
		_ = json.Unmarshal(raw, &gap)
	}

	fgColor := ""
	bold := false
	if raw, ok := n.Props["style"]; ok {
		var st map[string]any
		if json.Unmarshal(raw, &st) == nil {
			if c, ok := st["fg"].(string); ok {
				fgColor = c
			}
			if b, ok := st["bold"].(bool); ok {
				bold = b
			}
		}
	}

	metrics := layout.ComputeBox(r.w, r.h, layout.Box{
		Dir:     dir,
		Padding: padding,
		Gap:     gap,
		Border:  border,
	})

	if metrics.BorderInset > 0 {
		drawBorder(f, r.x, r.y, r.w, r.h, border, fgColor, bold)
	}

	contentRect := rect{
		x: r.x + metrics.BorderInset + metrics.PaddingInset,
		y: r.y + metrics.BorderInset + metrics.PaddingInset,
		w: metrics.ContentWidth,
		h: metrics.ContentHeight,
	}

	if len(n.Children) == 0 {
		return
	}

	if dir == layout.Column {
		allocated := tr.layoutColumnHeights(doc, n.Children, contentRect.h, gap)
		curY := contentRect.y
		for i, childID := range n.Children {
			chH := allocated[i]
			if chH > 0 && curY < contentRect.y+contentRect.h {
				tr.renderNode(f, doc, childID, rect{
					x: contentRect.x,
					y: curY,
					w: contentRect.w,
					h: chH,
				}, focusedID, inputs)
			}
			curY += chH + gap
		}
	} else {
		allocated := tr.layoutRowWidths(doc, n.Children, contentRect.w, gap)
		curX := contentRect.x
		for i, childID := range n.Children {
			chW := allocated[i]
			if chW > 0 && curX < contentRect.x+contentRect.w {
				tr.renderNode(f, doc, childID, rect{
					x: curX,
					y: contentRect.y,
					w: chW,
					h: contentRect.h,
				}, focusedID, inputs)
			}
			curX += chW + gap
		}
	}
}

func (tr *TerminalRenderer) layoutColumnHeights(doc document.Document, children []string, availableHeight, gap int) []int {
	n := len(children)
	res := make([]int, n)
	if n == 0 {
		return res
	}
	totalGap := layout.GapExtent(layout.Column, gap, n)
	avail := availableHeight - totalGap
	if avail <= 0 {
		return res
	}

	intrinsic := make([]int, n)
	flexCount := 0
	used := 0

	for i, id := range children {
		child := doc.Nodes[id]
		h := tr.intrinsicHeight(doc, child)
		if h < 0 {
			flexCount++
		} else {
			intrinsic[i] = h
			used += h
		}
	}

	remaining := avail - used
	if remaining < 0 {
		remaining = 0
	}

	for i := range children {
		if intrinsic[i] > 0 {
			res[i] = intrinsic[i]
		} else if flexCount > 0 {
			res[i] = remaining / flexCount
		} else {
			res[i] = avail / n
		}
		if res[i] < 1 && avail >= n {
			res[i] = 1
		}
	}
	return res
}

func (tr *TerminalRenderer) layoutRowWidths(doc document.Document, children []string, availableWidth, gap int) []int {
	n := len(children)
	res := make([]int, n)
	if n == 0 {
		return res
	}
	totalGap := layout.GapExtent(layout.Row, gap, n)
	avail := availableWidth - totalGap
	if avail <= 0 {
		return res
	}
	each := avail / n
	for i := range children {
		res[i] = each
	}
	return res
}

func (tr *TerminalRenderer) intrinsicHeight(doc document.Document, n document.Node) int {
	switch n.Type {
	case protocol.NodeText:
		text := ""
		if len(n.Props["text"]) > 0 {
			_ = json.Unmarshal(n.Props["text"], &text)
		}
		text += n.Text
		lines := strings.Count(text, "\n") + 1
		if lines < 1 {
			lines = 1
		}
		return lines
	case protocol.NodeInput:
		return 1
	case protocol.NodeActions:
		return 1
	case protocol.NodeProgress:
		return 1
	case protocol.NodeTable:
		rows := 0
		if raw, ok := n.Props["rows"]; ok {
			var r [][]string
			_ = json.Unmarshal(raw, &r)
			rows = len(r)
		}
		return rows + 2
	case protocol.NodeBox:
		return -1
	case protocol.NodeViewport:
		h := 10
		if raw, ok := n.Props["height"]; ok {
			_ = json.Unmarshal(raw, &h)
		}
		return h
	}
	return -1
}

func drawBorder(f *Frame, x, y, w, h int, border layout.Border, fg string, bold bool) {
	if w < 2 || h < 2 {
		return
	}
	type bChars struct {
		tl, tr, bl, br, h, v rune
	}
	var c bChars
	switch border {
	case layout.BorderRounded:
		c = bChars{tl: '╭', tr: '╮', bl: '╰', br: '╯', h: '─', v: '│'}
	default:
		c = bChars{tl: '┌', tr: '┐', bl: '└', br: '┘', h: '─', v: '│'}
	}

	f.Set(x, y, c.tl, fg, bold, false)
	f.Set(x+w-1, y, c.tr, fg, bold, false)
	f.Set(x, y+h-1, c.bl, fg, bold, false)
	f.Set(x+w-1, y+h-1, c.br, fg, bold, false)

	for col := x + 1; col < x+w-1; col++ {
		f.Set(col, y, c.h, fg, bold, false)
		f.Set(col, y+h-1, c.h, fg, bold, false)
	}
	for row := y + 1; row < y+h-1; row++ {
		f.Set(x, row, c.v, fg, bold, false)
		f.Set(x+w-1, row, c.v, fg, bold, false)
	}
}

func (tr *TerminalRenderer) renderText(f *Frame, n document.Node, r rect) {
	text := ""
	if len(n.Props["text"]) > 0 {
		_ = json.Unmarshal(n.Props["text"], &text)
	}
	text += n.Text
	fg := ""
	bold := false
	dim := false
	if raw, ok := n.Props["style"]; ok {
		var st map[string]any
		if json.Unmarshal(raw, &st) == nil {
			if c, ok := st["fg"].(string); ok {
				fg = c
			}
			if b, ok := st["bold"].(bool); ok {
				bold = b
			}
			if d, ok := st["dim"].(bool); ok {
				dim = d
			}
		}
	}

	lines := strings.Split(text, "\n")
	curY := r.y
	for _, line := range lines {
		if curY >= r.y+r.h {
			break
		}
		runes := []rune(line)
		if len(runes) > r.w {
			runes = runes[:r.w]
		}
		f.DrawString(r.x, curY, string(runes), fg, bold, dim)
		curY++
	}
}

func (tr *TerminalRenderer) renderInput(f *Frame, n document.Node, r rect, focused bool, currentValue string) {
	placeholder := ""
	if raw, ok := n.Props["placeholder"]; ok {
		_ = json.Unmarshal(raw, &placeholder)
	}
	display := currentValue
	fg := ""
	dim := false

	if display == "" && !focused {
		display = placeholder
		dim = true
		fg = "muted"
	}

	cursor := ""
	if focused {
		cursor = "█"
		fg = "primary"
	}

	content := fmt.Sprintf("[%s%s]", display, cursor)
	runes := []rune(content)
	if len(runes) > r.w {
		runes = runes[:r.w]
	}
	f.DrawString(r.x, r.y, string(runes), fg, focused, dim)
}

func (tr *TerminalRenderer) renderActions(f *Frame, n document.Node, r rect) {
	type item struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	var items []item
	if raw, ok := n.Props["items"]; ok {
		_ = json.Unmarshal(raw, &items)
	}
	curX := r.x
	for _, it := range items {
		btn := fmt.Sprintf("[%s: %s] ", it.Key, it.Label)
		btnRunes := []rune(btn)
		if curX+len(btnRunes) > r.x+r.w {
			break
		}
		f.DrawString(curX, r.y, btn, "primary", true, false)
		curX += len(btnRunes)
	}
}

func (tr *TerminalRenderer) renderProgress(f *Frame, n document.Node, r rect) {
	val := 0.0
	label := ""
	if raw, ok := n.Props["value"]; ok {
		_ = json.Unmarshal(raw, &val)
	}
	if raw, ok := n.Props["label"]; ok {
		_ = json.Unmarshal(raw, &label)
	}
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}

	percent := int(val * 100)
	pctStr := fmt.Sprintf("%3d%%", percent)

	prefix := "["
	suffix := "] " + pctStr
	if label != "" {
		suffix += " " + label
	}

	prefixRunes := []rune(prefix)
	suffixRunes := []rune(suffix)

	barWidth := r.w - len(prefixRunes) - len(suffixRunes)
	if barWidth < 5 {
		barWidth = 5
	}

	filled := int(float64(barWidth) * val)
	if filled > barWidth {
		filled = barWidth
	}
	empty := barWidth - filled
	if empty < 0 {
		empty = 0
	}

	barRunes := make([]rune, 0, len(prefixRunes)+barWidth+len(suffixRunes))
	barRunes = append(barRunes, prefixRunes...)
	for i := 0; i < filled; i++ {
		barRunes = append(barRunes, '█')
	}
	for i := 0; i < empty; i++ {
		barRunes = append(barRunes, '░')
	}
	barRunes = append(barRunes, suffixRunes...)

	if len(barRunes) > r.w {
		barRunes = barRunes[:r.w]
	}
	f.DrawString(r.x, r.y, string(barRunes), "primary", false, false)
}

func (tr *TerminalRenderer) renderTable(f *Frame, n document.Node, r rect) {
	type col struct {
		Title string `json:"title"`
		Width int    `json:"width"`
	}
	var cols []col
	var rows [][]string
	if raw, ok := n.Props["columns"]; ok {
		_ = json.Unmarshal(raw, &cols)
	}
	if raw, ok := n.Props["rows"]; ok {
		_ = json.Unmarshal(raw, &rows)
	}
	if len(cols) == 0 {
		return
	}

	curY := r.y
	header := ""
	divider := ""
	for i, c := range cols {
		w := c.Width
		if w <= 0 {
			w = 12
		}
		title := c.Title
		if utf8.RuneCountInString(title) > w {
			runes := []rune(title)
			title = string(runes[:w])
		}
		padded := fmt.Sprintf("%-*s", w, title)
		header += padded
		divider += strings.Repeat("─", w)
		if i < len(cols)-1 {
			header += " │ "
			divider += "─┼─"
		}
	}
	f.DrawString(r.x, curY, header, "primary", true, false)
	curY++
	if curY >= r.y+r.h {
		return
	}
	f.DrawString(r.x, curY, divider, "muted", false, false)
	curY++

	for _, row := range rows {
		if curY >= r.y+r.h {
			break
		}
		rowStr := ""
		for i, c := range cols {
			w := c.Width
			if w <= 0 {
				w = 12
			}
			val := ""
			if i < len(row) {
				val = row[i]
			}
			if utf8.RuneCountInString(val) > w {
				runes := []rune(val)
				val = string(runes[:w])
			}
			padded := fmt.Sprintf("%-*s", w, val)
			rowStr += padded
			if i < len(cols)-1 {
				rowStr += " │ "
			}
		}
		f.DrawString(r.x, curY, rowStr, "", false, false)
		curY++
	}
}

func (tr *TerminalRenderer) renderViewport(f *Frame, doc document.Document, n document.Node, r rect, focusedID string, inputs map[string]string) {
	vpHeight := layout.ClampViewportHeight(r.h, r.h)
	if raw, ok := n.Props["height"]; ok {
		var req int
		_ = json.Unmarshal(raw, &req)
		vpHeight = layout.ClampViewportHeight(req, r.h)
	}
	vpRect := rect{x: r.x, y: r.y, w: r.w, h: vpHeight}
	for _, childID := range n.Children {
		tr.renderNode(f, doc, childID, vpRect, focusedID, inputs)
	}
}
