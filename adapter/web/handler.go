package web

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strconv"
	"strings"

	"a2ui/document"
	"a2ui/engine"
	"a2ui/protocol"
)

// Controller is the renderer-neutral interaction boundary consumed by the Web
// proof renderer. ipc.Client satisfies this interface on supported local Unix
// platforms.
type Controller interface {
	Snapshot() engine.PresentationSnapshot
	Focus(id string) error
	SetInput(id, value string) error
	Submit(id string) error
	SelectTableRow(id, rowID string) error
	ActivateTableSelection(id string) error
	InvokeAction(id, action string, args json.RawMessage) error
	AcknowledgePublication(generation uint64) error
}

type handler struct {
	controller Controller
}

func NewHandler(controller Controller) http.Handler {
	return &handler{controller: controller}
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		h.serveIndex(w, r)
	case "/interaction":
		h.serveInteraction(w, r)
	case "/published":
		h.servePublished(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.controller == nil {
		http.Error(w, "renderer controller unavailable", http.StatusServiceUnavailable)
		return
	}
	snapshot := h.controller.Snapshot()
	var body strings.Builder
	body.WriteString("<!doctype html><html><head><meta charset=\"utf-8\"><title>AIR</title></head><body>")
	fmt.Fprintf(&body, "<main data-air-generation=\"%d\">", snapshot.PublicationGeneration)
	renderNode(&body, snapshot, "root")
	body.WriteString("</main>")
	if snapshot.PublicationPending && snapshot.PublicationGeneration > 0 {
		fmt.Fprintf(&body, `<script>addEventListener("DOMContentLoaded",()=>fetch("/published",{method:"POST",headers:{"Content-Type":"application/x-www-form-urlencoded"},body:"generation=%d"}));</script>`, snapshot.PublicationGeneration)
	}
	body.WriteString("</body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(body.String()))
}

func (h *handler) serveInteraction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.controller == nil {
		http.Error(w, "renderer controller unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	id := r.Form.Get("id")
	var err error
	switch r.Form.Get("type") {
	case "focus":
		err = h.controller.Focus(id)
	case "input_set":
		err = h.controller.SetInput(id, r.Form.Get("value"))
	case "input_submit":
		if setErr := h.controller.SetInput(id, r.Form.Get("value")); setErr != nil {
			err = setErr
		} else {
			err = h.controller.Submit(id)
		}
	case "table_select":
		err = h.controller.SelectTableRow(id, r.Form.Get("row_id"))
	case "table_activate":
		err = h.controller.ActivateTableSelection(id)
	case "action_invoke":
		args := json.RawMessage(r.Form.Get("args"))
		if len(args) == 0 {
			args = nil
		}
		err = h.controller.InvokeAction(id, r.Form.Get("action"), args)
	default:
		http.Error(w, "unknown semantic interaction", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handler) servePublished(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.controller == nil {
		http.Error(w, "renderer controller unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	generation, err := strconv.ParseUint(r.Form.Get("generation"), 10, 64)
	if err != nil || generation == 0 {
		http.Error(w, "invalid publication generation", http.StatusBadRequest)
		return
	}
	if err := h.controller.AcknowledgePublication(generation); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func renderNode(out *strings.Builder, snapshot engine.PresentationSnapshot, id string) {
	n, ok := snapshot.Document.Nodes[id]
	if !ok {
		return
	}
	switch n.Type {
	case protocol.NodeBox:
		fmt.Fprintf(out, "<section data-air-node=\"%s\">", html.EscapeString(n.ID))
		renderChildren(out, snapshot, n)
		out.WriteString("</section>")
	case protocol.NodeViewport:
		fmt.Fprintf(out, "<div data-air-node=\"%s\">", html.EscapeString(n.ID))
		renderChildren(out, snapshot, n)
		out.WriteString("</div>")
	case protocol.NodeText:
		text := stringProp(n, "text")
		if text == "" {
			text = n.Text
		}
		fmt.Fprintf(out, "<p data-air-node=\"%s\">%s</p>", html.EscapeString(n.ID), html.EscapeString(text))
	case protocol.NodeInput:
		value := snapshot.InputValues[n.ID]
		fmt.Fprintf(out, `<form method="post" action="/interaction" data-air-node="%s"><input type="hidden" name="type" value="input_submit"><input type="hidden" name="id" value="%s"><input name="value" value="%s"><button type="submit">Submit</button></form>`, html.EscapeString(n.ID), html.EscapeString(n.ID), html.EscapeString(value))
	case protocol.NodeActions:
		renderActions(out, n)
	case protocol.NodeTable:
		renderTable(out, n)
	case protocol.NodeProgress:
		renderProgress(out, n)
	}
}

func renderChildren(out *strings.Builder, snapshot engine.PresentationSnapshot, n document.Node) {
	for _, child := range n.Children {
		renderNode(out, snapshot, child)
	}
}

func stringProp(n document.Node, key string) string {
	var value string
	_ = json.Unmarshal(n.Props[key], &value)
	return value
}

func renderActions(out *strings.Builder, n document.Node) {
	var items []struct {
		Label  string          `json:"label"`
		Action string          `json:"action"`
		Args   json.RawMessage `json:"args"`
	}
	_ = json.Unmarshal(n.Props["items"], &items)
	fmt.Fprintf(out, "<div data-air-node=\"%s\">", html.EscapeString(n.ID))
	for _, item := range items {
		fmt.Fprintf(out, `<form method="post" action="/interaction"><input type="hidden" name="type" value="action_invoke"><input type="hidden" name="id" value="%s"><input type="hidden" name="action" value="%s"><input type="hidden" name="args" value="%s"><button type="submit">%s</button></form>`, html.EscapeString(n.ID), html.EscapeString(item.Action), html.EscapeString(string(item.Args)), html.EscapeString(item.Label))
	}
	out.WriteString("</div>")
}

func renderTable(out *strings.Builder, n document.Node) {
	var columns []struct {
		Title string `json:"title"`
	}
	var rows [][]string
	var rowIDs []string
	_ = json.Unmarshal(n.Props["columns"], &columns)
	_ = json.Unmarshal(n.Props["rows"], &rows)
	_ = json.Unmarshal(n.Props["row_ids"], &rowIDs)
	fmt.Fprintf(out, "<table data-air-node=\"%s\"><thead><tr>", html.EscapeString(n.ID))
	for _, column := range columns {
		fmt.Fprintf(out, "<th>%s</th>", html.EscapeString(column.Title))
	}
	out.WriteString("</tr></thead><tbody>")
	for index, row := range rows {
		rowID := ""
		if index < len(rowIDs) {
			rowID = rowIDs[index]
		}
		fmt.Fprintf(out, "<tr data-air-row-id=\"%s\">", html.EscapeString(rowID))
		for _, cell := range row {
			fmt.Fprintf(out, "<td>%s</td>", html.EscapeString(cell))
		}
		if rowID != "" {
			fmt.Fprintf(out, `<td><form method="post" action="/interaction"><input type="hidden" name="type" value="table_select"><input type="hidden" name="id" value="%s"><button type="submit" name="row_id" value="%s">Select</button></form></td>`, html.EscapeString(n.ID), html.EscapeString(rowID))
		}
		out.WriteString("</tr>")
	}
	out.WriteString("</tbody></table>")
}

func renderProgress(out *strings.Builder, n document.Node) {
	var value float64
	var max float64
	_ = json.Unmarshal(n.Props["value"], &value)
	_ = json.Unmarshal(n.Props["max"], &max)
	if max <= 0 {
		max = 1
	}
	fmt.Fprintf(out, `<progress data-air-node="%s" value="%g" max="%g"></progress>`, html.EscapeString(n.ID), value, max)
}
