package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/Ostrovsky42/agent-interaction-runtime/document"
	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	a2runtime "github.com/Ostrovsky42/agent-interaction-runtime/runtime"
	"github.com/Ostrovsky42/agent-interaction-runtime/session"
	"github.com/Ostrovsky42/agent-interaction-runtime/transport/mcp"
	"github.com/Ostrovsky42/agent-interaction-runtime/wire"
)

// Runner manages a complete A2UI node: Engine, Session state machine, Action
// registry, and Terminal Renderer. It can accept messages via MCP JSON-RPC,
// NDJSON streaming envelopes, or HTTP POST requests (2026-07-28 spec).
type Runner struct {
	mu           sync.Mutex
	Session      *session.Session
	Engine       *engine.Engine
	Actions      *a2runtime.ActionRegistry
	Renderer     *TerminalRenderer
	frames       []*Frame
	lastFrame    *Frame
	publishedRev []uint64
	onFrameHook  func(frame *Frame, rev uint64)
}

// NewRunner creates a new integration runner.
func NewRunner(sessionID string, limits protocol.Limits, termW, termH int, actions *a2runtime.ActionRegistry) *Runner {
	effLimits := protocol.EffectiveLimits(limits)
	if actions == nil {
		actions = engine.NewNoopActions()
	}
	eng := engine.New(effLimits, effLimits.MaxPendingEvents, actions)
	sess := session.New(sessionID, effLimits)
	rend := NewTerminalRenderer(termW, termH)

	return &Runner{
		Session:  sess,
		Engine:   eng,
		Actions:  actions,
		Renderer: rend,
	}
}

// SetFrameHook attaches a callback that is invoked whenever a commit publication occurs.
func (r *Runner) SetFrameHook(hook func(frame *Frame, rev uint64)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onFrameHook = hook
}

// LastFrame returns the most recently rendered frame, or nil if none rendered yet.
func (r *Runner) LastFrame() *Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastFrame
}

// Frames returns all rendered frames in chronological order.
func (r *Runner) Frames() []*Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*Frame, len(r.frames))
	copy(out, r.frames)
	return out
}

// Document returns the current authoritative document.
func (r *Runner) Document() document.Document {
	return r.Engine.Document()
}

// HandleEnvelope processes an incoming raw A2UI envelope.
func (r *Runner) HandleEnvelope(env protocol.Envelope) (*protocol.Envelope, *protocol.Error) {
	switch env.Kind {
	case protocol.KindHello:
		var h protocol.Hello
		if perr := wire.StrictUnmarshal(env.Payload, &h); perr != nil {
			return nil, perr
		}
		ack, perr := r.Session.Negotiate(h)
		if perr != nil {
			return nil, perr
		}
		ackPayload, err := json.Marshal(ack)
		if err != nil {
			return nil, protocol.NewError("transport.encode_error", err.Error())
		}
		resp := protocol.Envelope{
			V:       protocol.Version,
			Session: r.Session.ID(),
			Kind:    protocol.KindHelloAck,
			Payload: ackPayload,
		}
		return &resp, nil

	case protocol.KindOperation:
		duplicate, perr := r.Session.AcceptMutation(env.Seq)
		if perr != nil {
			return nil, perr
		}
		if duplicate {
			return nil, nil
		}

		var op protocol.Operation
		if perr := wire.StrictUnmarshal(env.Payload, &op); perr != nil {
			return nil, perr
		}
		if op.Seq != int64(env.Seq) {
			return nil, protocol.NewError("protocol.sequence_mismatch", fmt.Sprintf("envelope seq %d != operation seq %d", env.Seq, op.Seq))
		}

		if applyErr := r.Engine.Apply(op); applyErr != nil {
			return nil, applyErr
		}

		// Check if commit or dirty requires publication barrier
		if op.Op == protocol.OpCommit || r.Engine.NeedsPublish() {
			r.publishFrame()
		}
		return nil, nil

	case protocol.KindTelemetry:
		return nil, nil

	default:
		return nil, protocol.NewError("protocol.unsupported_kind", fmt.Sprintf("unsupported envelope kind %q", env.Kind))
	}
}

func (r *Runner) renderFrameLocal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := r.Engine.PresentationSnapshot()
	frame := r.Renderer.Render(snapshot.Document, snapshot.FocusedID, snapshot.InputValues)
	r.frames = append(r.frames, frame)
	r.lastFrame = frame
}

func (r *Runner) publishFrame() {
	r.mu.Lock()
	defer r.mu.Unlock()

	snapshot := r.Engine.PresentationSnapshot()
	frame := r.Renderer.Render(snapshot.Document, snapshot.FocusedID, snapshot.InputValues)
	r.frames = append(r.frames, frame)
	r.lastFrame = frame
	r.publishedRev = append(r.publishedRev, snapshot.Document.Revision)

	_ = r.Engine.Publish()

	if r.onFrameHook != nil {
		r.onFrameHook(frame, snapshot.Document.Revision)
	}
}

// HandleMCPMessage strictly decodes and processes an MCP JSON-RPC message.
func (r *Runner) HandleMCPMessage(msg mcp.Message) (*mcp.Message, *protocol.Error) {
	env, perr := mcp.EnvelopeFromMessage(msg)
	if perr != nil {
		return nil, perr
	}

	respEnv, perr := r.HandleEnvelope(env)
	if perr != nil {
		return nil, perr
	}
	if respEnv == nil {
		return nil, nil
	}

	var respMsg mcp.Message
	var err error
	if len(msg.ID) > 0 {
		respMsg, err = mcp.Request(msg.ID, *respEnv)
	} else {
		respMsg, err = mcp.Notification(*respEnv)
	}
	if err != nil {
		return nil, protocol.NewError("mcp.encode_failed", err.Error())
	}
	return &respMsg, nil
}

// ServeHTTP implements http.Handler conforming to MCP 2026-07-28 stateless HTTP spec.
func (r *Runner) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	msg, perr := mcp.DecodeMessage(body)
	if perr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(mcp.RPCError{
			Code:    -32700,
			Message: perr.Message,
		})
		return
	}

	if perr := mcp.ValidateHTTPHeaders(req.Header, msg); perr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(mcp.RPCError{
			Code:    -32600,
			Message: perr.Message,
		})
		return
	}

	respMsg, rerr := r.HandleMCPMessage(msg)
	if rerr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(mcp.RPCError{
			Code:    -32000,
			Message: rerr.Message,
		})
		return
	}

	if respMsg != nil {
		for k, v := range mcp.HTTPHeaders(*respMsg) {
			w.Header()[k] = v
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(respMsg)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// TypeInput simulates user typing into an input field.
func (r *Runner) TypeInput(nodeID, value string) *protocol.Error {
	if err := r.Engine.SetInput(nodeID, value); err != nil {
		return err
	}
	// Local editing requires a visible redraw but no protocol publication
	// acknowledgement. Keep rendering and Publish() as separate operations.
	r.renderFrameLocal()
	return nil
}

// SubmitInput triggers the submit event for an input field.
func (r *Runner) SubmitInput(nodeID string) *protocol.Error {
	return r.Engine.Submit(nodeID)
}

// PressKey triggers an action bound to the given key.
func (r *Runner) PressKey(ctx context.Context, key string) *protocol.Error {
	return r.Engine.HandleKey(ctx, key)
}

// DrainEvents drains all pending events queued in the engine.
func (r *Runner) DrainEvents() []protocol.Event {
	var events []protocol.Event
	for {
		ev, ok := r.Engine.NextEvent()
		if !ok {
			break
		}
		events = append(events, ev)
	}
	return events
}

// DrainMCPNotifications returns pending events formatted as MCP notifications.
func (r *Runner) DrainMCPNotifications() ([]mcp.Message, error) {
	events := r.DrainEvents()
	messages := make([]mcp.Message, 0, len(events))
	for _, ev := range events {
		payload, err := json.Marshal(ev)
		if err != nil {
			return nil, err
		}
		env := protocol.Envelope{
			V:       protocol.Version,
			Session: r.Session.ID(),
			Kind:    protocol.KindEvent,
			Seq:     ev.Seq,
			Payload: payload,
		}
		msg, err := mcp.Notification(env)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	return messages, nil
}
