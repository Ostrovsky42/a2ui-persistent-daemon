package daemon

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	"github.com/Ostrovsky42/agent-interaction-runtime/transport/mcp"
)

// HandleMCPMessage bridges the existing public A2UI MCP envelope contract into
// the daemon-owned Session/Engine. It deliberately performs no rendering and
// never acknowledges publication on behalf of a terminal client.
func (d *Daemon) HandleMCPMessage(msg mcp.Message) (*mcp.Message, *protocol.Error) {
	if msg.Method == "github.com/Ostrovsky42/agent-interaction-runtime/drain_events" || msg.Method == "github.com/Ostrovsky42/agent-interaction-runtime/wait_event" {
		timeout := 0 * time.Second
		if msg.Method == "github.com/Ostrovsky42/agent-interaction-runtime/wait_event" {
			timeout = 30 * time.Second
			var params struct {
				TimeoutMs int `json:"timeout_ms"`
			}
			if len(msg.Params) > 0 {
				_ = json.Unmarshal(msg.Params, &params)
				if params.TimeoutMs > 0 {
					timeout = time.Duration(params.TimeoutMs) * time.Millisecond
				}
			}
		}
		events := d.WaitEvents(context.Background(), timeout)
		if events == nil {
			events = []protocol.Event{}
		}
		resRaw, err := json.Marshal(map[string]any{"events": events})
		if err != nil {
			return nil, protocol.NewError("mcp.encode_failed", err.Error())
		}
		return &mcp.Message{
			JSONRPC: "2.0",
			ID:      msg.ID,
			Result:  resRaw,
		}, nil
	}

	env, perr := mcp.EnvelopeFromMessage(msg)
	if perr != nil {
		return nil, perr
	}
	respEnv, perr := d.HandleEnvelope(env)
	if perr != nil {
		return nil, perr
	}
	if respEnv == nil {
		return nil, nil
	}
	var (
		resp mcp.Message
		err  error
	)
	if len(msg.ID) > 0 {
		resp, err = mcp.Request(msg.ID, *respEnv)
	} else {
		resp, err = mcp.Notification(*respEnv)
	}
	if err != nil {
		return nil, protocol.NewError("mcp.encode_failed", err.Error())
	}
	return &resp, nil
}

// DrainMCPNotifications translates already-classified semantic Engine events
// into the existing transport-independent MCP bridge notifications.
func (d *Daemon) DrainMCPNotifications() ([]mcp.Message, error) {
	events := d.DrainEvents()
	out := make([]mcp.Message, 0, len(events))
	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		env := protocol.Envelope{
			V:       protocol.Version,
			Session: d.Session.ID(),
			Kind:    protocol.KindEvent,
			Seq:     ev.Seq,
			Payload: payload,
		}
		msg, err := mcp.Notification(env)
		if err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, nil
}

// ServeHTTP exposes the stateless MCP Streamable HTTP bridge as well as
// ergonomics GET /status and GET /events endpoints.
func (d *Daemon) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodGet {
		switch req.URL.Path {
		case "/status":
			d.serveStatus(w, req)
			return
		case "/events":
			d.serveEvents(w, req)
			return
		}
	}

	if req.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	max := int64(d.limits.MaxMessageBytes)
	if max < 1 {
		max = int64(protocol.DefaultLimits().MaxMessageBytes)
	}
	req.Body = http.MaxBytesReader(w, req.Body, max)
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	msg, perr := mcp.DecodeMessage(body)
	if perr != nil {
		writeMCPError(w, http.StatusBadRequest, -32700, perr.Message)
		return
	}
	if perr := mcp.ValidateHTTPHeaders(req.Header, msg); perr != nil {
		writeMCPError(w, http.StatusBadRequest, -32600, perr.Message)
		return
	}
	resp, perr := d.HandleMCPMessage(msg)
	if perr != nil {
		writeMCPError(w, http.StatusUnprocessableEntity, -32000, perr.Message)
		return
	}
	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	for key, values := range mcp.HTTPHeaders(*resp) {
		w.Header()[key] = values
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (d *Daemon) serveStatus(w http.ResponseWriter, req *http.Request) {
	doc := d.Engine.Document()
	curGen, pending := d.Engine.PublicationGeneration()
	status := map[string]any{
		"session":         d.Session.ID(),
		"revision":        doc.Revision,
		"nodes":           len(doc.Nodes),
		"has_client":      d.HasActiveClient(),
		"generation":      curGen,
		"pending_publish": pending,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(status)
}

func (d *Daemon) serveEvents(w http.ResponseWriter, req *http.Request) {
	timeout := 30 * time.Second
	if q := req.URL.Query().Get("timeout"); q != "" {
		if dur, err := time.ParseDuration(q); err == nil && dur >= 0 {
			timeout = dur
		}
	}
	events := d.WaitEvents(req.Context(), timeout)
	if events == nil {
		events = []protocol.Event{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(events)
}

func writeMCPError(w http.ResponseWriter, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(mcp.RPCError{Code: code, Message: message})
}
