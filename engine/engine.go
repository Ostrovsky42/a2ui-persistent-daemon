package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/document"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	a2runtime "github.com/Ostrovsky42/agent-interaction-runtime/runtime"
)

type PresentationSnapshot struct {
	Document              document.Document                   `json:"document"`
	FocusedID             string                              `json:"focused_id,omitempty"`
	InputValues           map[string]string                   `json:"input_values"`
	TableSelections       map[string]a2runtime.TableSelection `json:"table_selections"`
	Bindings              map[string]a2runtime.Binding        `json:"bindings"`
	RenderGeneration      uint64                              `json:"render_generation"`
	PublicationGeneration uint64                              `json:"publication_generation"`
	PublicationPending    bool                                `json:"publication_pending"`
}

type commitRequest struct {
	through uint64
	frame   string
}

type Engine struct {
	mu                    sync.Mutex
	doc                   document.Document
	state                 *a2runtime.State
	broker                *a2runtime.EventBroker
	actions               *a2runtime.ActionRegistry
	limits                protocol.Limits
	pendingCommits        []commitRequest
	maxPendingCommits     int
	publicationGeneration uint64
}

func New(limits protocol.Limits, eventCapacity int, actions *a2runtime.ActionRegistry) *Engine {
	limits = protocol.EffectiveLimits(limits)
	if eventCapacity < 1 {
		eventCapacity = limits.MaxPendingEvents
	}
	if eventCapacity > limits.MaxPendingEvents {
		eventCapacity = limits.MaxPendingEvents
	}
	maxPendingCommits := limits.MaxPendingMutations
	if eventCapacity < maxPendingCommits {
		maxPendingCommits = eventCapacity
	}
	if actions == nil {
		actions = NewNoopActions()
	}
	return &Engine{doc: document.New(), state: a2runtime.NewState(), broker: a2runtime.NewEventBroker(eventCapacity), actions: actions, limits: limits, maxPendingCommits: maxPendingCommits}
}
func NewNoopActions() *a2runtime.ActionRegistry {
	return a2runtime.NewActionRegistry(5*time.Second, protocol.DefaultLimits().MaxInflightActions)
}

func (e *Engine) Document() document.Document { e.mu.Lock(); defer e.mu.Unlock(); return e.doc.Clone() }
func (e *Engine) FocusedID() string           { e.mu.Lock(); defer e.mu.Unlock(); return e.state.FocusedID }

func (e *Engine) InputValue(id string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.InputValues[id]
}

func (e *Engine) InputValues() map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]string, len(e.state.InputValues))
	for k, v := range e.state.InputValues {
		out[k] = v
	}
	return out
}

func (e *Engine) PresentationSnapshot() PresentationSnapshot {
	e.mu.Lock()
	defer e.mu.Unlock()
	inputs := make(map[string]string, len(e.state.InputValues))
	for id, value := range e.state.InputValues {
		inputs[id] = value
	}
	selections := make(map[string]a2runtime.TableSelection, len(e.state.TableSelections))
	for id, selection := range e.state.TableSelections {
		selections[id] = selection
	}
	bindings := make(map[string]a2runtime.Binding, len(e.state.Bindings))
	for key, binding := range e.state.Bindings {
		binding.Args = append(json.RawMessage(nil), binding.Args...)
		bindings[key] = binding
	}
	return PresentationSnapshot{
		Document:              e.doc.Clone(),
		FocusedID:             e.state.FocusedID,
		InputValues:           inputs,
		TableSelections:       selections,
		Bindings:              bindings,
		RenderGeneration:      e.state.RenderGeneration,
		PublicationGeneration: e.publicationGeneration,
		PublicationPending:    e.needsPublishLocked(),
	}
}

func (e *Engine) emitError(op protocol.Operation, perr *protocol.Error) *protocol.Error {
	ev := protocol.Event{Ev: "error", Code: perr.Code, Msg: perr.Message, ID: op.ID, RelatedSeq: uint64(max64(op.Seq, 0))}
	if berr := e.broker.Enqueue(ev, a2runtime.EventCritical); berr != nil {
		return berr
	}
	return perr
}

func (e *Engine) Apply(op protocol.Operation) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	beforePublication := e.state.PublicationGeneration
	next, eff, perr := document.Apply(e.doc, op, e.limits)
	if perr != nil {
		return e.emitError(op, perr)
	}
	if eff.Commit && len(e.pendingCommits) >= e.maxPendingCommits {
		return e.emitError(op, protocol.NewError("runtime.backpressure_exceeded", "pending commit limit reached"))
	}
	if rerr := e.state.Reconcile(e.doc, next, op, eff); rerr != nil {
		return e.emitError(op, rerr)
	}
	e.doc = next
	if eff.Commit {
		e.pendingCommits = append(e.pendingCommits, commitRequest{through: uint64(max64(op.Seq, 0)), frame: op.Frame})
	}
	if e.state.PublicationGeneration != beforePublication || eff.Commit {
		e.publicationGeneration++
	}
	return nil
}

// NeedsPublish reports whether the renderer has a dirty document revision or a
// pending commit barrier that still requires a publication acknowledgement.
func (e *Engine) needsPublishLocked() bool {
	return e.state.NeedsPublish() || len(e.pendingCommits) > 0
}

func (e *Engine) NeedsPublish() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.needsPublishLocked()
}

// PublicationGeneration returns the current renderer publication token and
// whether that exact semantic frame still awaits visibility acknowledgement.
func (e *Engine) PublicationGeneration() (uint64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.publicationGeneration, e.needsPublishLocked()
}

func (e *Engine) publishLocked() *protocol.Error {
	if !e.needsPublishLocked() {
		return nil
	}
	revision := e.state.DirtyRevision
	if revision < e.state.PublishedRevision {
		revision = e.state.PublishedRevision
	}
	events := make([]protocol.Event, 0, len(e.pendingCommits))
	for _, req := range e.pendingCommits {
		events = append(events, protocol.Event{Ev: "committed", Revision: revision, ThroughSeq: req.through, Frame: req.frame})
	}
	if berr := e.broker.EnqueueCriticalBatch(events); berr != nil {
		return berr
	}
	e.state.Publish()
	e.pendingCommits = e.pendingCommits[:0]
	return nil
}

// Publish MUST be called by the renderer only after it has made its current
// snapshot visible. It preserves the standalone V1-V3 API by acknowledging the
// current publication generation.
func (e *Engine) Publish() *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.publishLocked()
}

// PublishGeneration acknowledges visibility only when generation is the exact
// current publication token. A stale token can never publish a newer semantic
// frame. The bool reports whether a pending frame was actually published.
func (e *Engine) PublishGeneration(generation uint64) (bool, *protocol.Error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if generation != e.publicationGeneration || !e.needsPublishLocked() {
		return false, nil
	}
	if perr := e.publishLocked(); perr != nil {
		return false, perr
	}
	return true, nil
}

func (e *Engine) NextEvent() (protocol.Event, bool) { return e.broker.Next() }

func (e *Engine) SetInput(id, value string) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	n, ok := e.doc.Nodes[id]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", id))
	}
	if n.Type != protocol.NodeInput {
		return protocol.NewError("runtime.not_input", fmt.Sprintf("node %q is not input", id))
	}
	e.state.SetInputValue(id, value)
	return nil
}

func (e *Engine) SelectedTableRow(id string) (a2runtime.TableSelection, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.TableSelection(id)
}

func (e *Engine) SetTableSelection(id string, index int) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.SetTableSelection(e.doc, id, index)
}

func (e *Engine) SetTableSelectionByRowID(id, rowID string) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.SetTableSelectionByRowID(e.doc, id, rowID)
}

func (e *Engine) MoveTableSelection(id string, delta int) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.MoveTableSelection(e.doc, id, delta)
}

func (e *Engine) ActivateTableSelection(id string) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	n, ok := e.doc.Nodes[id]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", id))
	}
	if n.Type != protocol.NodeTable {
		return protocol.NewError("runtime.not_table", fmt.Sprintf("node %q is not table", id))
	}
	var selectable bool
	_ = json.Unmarshal(n.Props["selectable"], &selectable)
	if !selectable {
		return protocol.NewError("runtime.not_selectable", fmt.Sprintf("table %q is not selectable", id))
	}
	selection, exists := e.state.TableSelection(id)
	if !exists {
		return nil
	}
	var action string
	_ = json.Unmarshal(n.Props["action"], &action)
	ev := protocol.Event{Ev: "select", ID: id, Row: selection.Index, RowID: selection.RowID, Action: action}
	return e.broker.Enqueue(ev, a2runtime.EventCritical)
}

func (e *Engine) Submit(id string) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	n, ok := e.doc.Nodes[id]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", id))
	}
	if n.Type != protocol.NodeInput {
		return protocol.NewError("runtime.not_input", fmt.Sprintf("node %q is not input", id))
	}
	var action string
	_ = json.Unmarshal(n.Props["action"], &action)
	ev := protocol.Event{Ev: "submit", ID: id, Action: action, Value: e.state.InputValues[id]}
	return e.broker.Enqueue(ev, a2runtime.EventCritical)
}

func (e *Engine) Focus(id string) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state.SetLocalFocus(e.doc, id)
}

func (e *Engine) FocusableIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return a2runtime.FocusableIDs(e.doc)
}

func (e *Engine) HasKeyBinding(key string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.state.Bindings[key]
	return ok
}

func equivalentArgs(a, b json.RawMessage) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) == 0 {
		a = json.RawMessage("null")
	}
	if len(b) == 0 {
		b = json.RawMessage("null")
	}
	var av, bv any
	if json.Unmarshal(a, &av) != nil || json.Unmarshal(b, &bv) != nil {
		return false
	}
	return reflect.DeepEqual(av, bv)
}

func (e *Engine) dispatchBinding(ctx context.Context, binding a2runtime.Binding) *protocol.Error {
	ev := e.actions.Dispatch(ctx, binding.NodeID, binding.Action, append(json.RawMessage(nil), binding.Args...))
	return e.broker.Enqueue(ev, a2runtime.EventCritical)
}

// InvokeAction dispatches only an action that is currently exposed by the
// semantic Document/runtime binding projection. A renderer cannot use this API
// to reach an arbitrary registered host capability.
func (e *Engine) InvokeAction(ctx context.Context, nodeID, action string, args json.RawMessage) *protocol.Error {
	e.mu.Lock()
	var matched a2runtime.Binding
	found := false
	for _, binding := range e.state.Bindings {
		if binding.NodeID == nodeID && binding.Action == action && equivalentArgs(binding.Args, args) {
			matched = binding
			matched.Args = append(json.RawMessage(nil), binding.Args...)
			found = true
			break
		}
	}
	e.mu.Unlock()
	if !found {
		return protocol.NewError("action.not_permitted", "action is not exposed by the current semantic surface")
	}
	return e.dispatchBinding(ctx, matched)
}

func (e *Engine) HandleKey(ctx context.Context, key string) *protocol.Error {
	e.mu.Lock()
	binding, ok := e.state.Bindings[key]
	if ok {
		binding.Args = append(json.RawMessage(nil), binding.Args...)
	}
	e.mu.Unlock()
	if !ok {
		return nil
	}
	return e.dispatchBinding(ctx, binding)
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
