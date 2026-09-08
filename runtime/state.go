package runtime

import (
	"encoding/json"
	"reflect"

	"github.com/Ostrovsky42/agent-interaction-runtime/document"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

type Binding struct {
	NodeID string          `json:"node_id"`
	Key    string          `json:"key"`
	Label  string          `json:"label"`
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args,omitempty"`
}

type State struct {
	FocusedID             string
	InputValues           map[string]string
	TableSelections       map[string]TableSelection
	Bindings              map[string]Binding
	DirtyRevision         uint64
	PublishedRevision     uint64
	RenderGeneration      uint64
	PublicationGeneration uint64
	PublishedGeneration   uint64
}

func NewState() *State {
	return &State{InputValues: map[string]string{}, TableSelections: map[string]TableSelection{}, Bindings: map[string]Binding{}}
}

func (s *State) clone() *State {
	out := &State{FocusedID: s.FocusedID, DirtyRevision: s.DirtyRevision, PublishedRevision: s.PublishedRevision, RenderGeneration: s.RenderGeneration, PublicationGeneration: s.PublicationGeneration, PublishedGeneration: s.PublishedGeneration, InputValues: map[string]string{}, TableSelections: map[string]TableSelection{}, Bindings: map[string]Binding{}}
	for k, v := range s.InputValues {
		out.InputValues[k] = v
	}
	for k, v := range s.TableSelections {
		out.TableSelections[k] = v
	}
	for k, v := range s.Bindings {
		out.Bindings[k] = v
	}
	return out
}

func propString(n document.Node, key string) string {
	var v string
	if raw := n.Props[key]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}
func propBool(n document.Node, key string) bool {
	var v bool
	if raw := n.Props[key]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &v)
	}
	return v
}

func focusable(n document.Node) bool {
	return n.Type == protocol.NodeInput ||
		(n.Type == protocol.NodeTable && propBool(n, "selectable")) ||
		(n.Type == protocol.NodeViewport && propBool(n, "scrollable"))
}

func (s *State) Reconcile(prev, next document.Document, op protocol.Operation, eff document.Effect) *protocol.Error {
	hadFocus := s.FocusedID != ""
	cand := s.clone()
	for _, id := range eff.Removed {
		delete(cand.InputValues, id)
		if cand.FocusedID == id {
			cand.FocusedID = ""
		}
	}
	if eff.TypeChanged {
		delete(cand.InputValues, op.ID)
		if cand.FocusedID == op.ID {
			cand.FocusedID = ""
		}
	}

	for id, n := range next.Nodes {
		if n.Type != protocol.NodeInput {
			continue
		}
		old, existed := prev.Nodes[id]
		if !existed || old.Type != protocol.NodeInput {
			cand.InputValues[id] = propString(n, "value")
		} else if eff.Policy.ForceInputValue && id == op.ID {
			cand.InputValues[id] = propString(n, "value")
		} else if _, ok := cand.InputValues[id]; !ok {
			cand.InputValues[id] = propString(n, "value")
		}
	}

	if op.Op == protocol.OpFocus {
		n, ok := next.Nodes[op.ID]
		if !ok {
			return protocol.NewError("runtime.node_not_found", "focus target missing")
		}
		if !focusable(n) {
			return protocol.NewError("runtime.not_focusable", "focus target is not interactive")
		}
		cand.FocusedID = op.ID
	}
	if cand.FocusedID != "" {
		n, ok := next.Nodes[cand.FocusedID]
		if !ok || !focusable(n) {
			cand.FocusedID = ""
		}
	}
	if hadFocus && cand.FocusedID == "" && focusNeedsFallback(op, eff) {
		cand.FocusedID = firstFocusable(next, "root")
	}

	cand.reconcileTableSelections(next)

	bindings, berr := buildBindings(next)
	if berr != nil {
		return berr
	}
	cand.Bindings = bindings
	if eff.Changed {
		cand.DirtyRevision = next.Revision
	}
	projectionChanged := eff.Changed || cand.FocusedID != s.FocusedID || !reflect.DeepEqual(cand.InputValues, s.InputValues) || !reflect.DeepEqual(cand.TableSelections, s.TableSelections) || !reflect.DeepEqual(cand.Bindings, s.Bindings)
	if projectionChanged {
		cand.RenderGeneration = s.RenderGeneration + 1
		cand.PublicationGeneration = s.PublicationGeneration + 1
	}
	*s = *cand
	return nil
}

func (s *State) SetLocalFocus(d document.Document, id string) *protocol.Error {
	n, ok := d.Nodes[id]
	if !ok {
		return protocol.NewError("runtime.node_not_found", "focus target missing")
	}
	if !focusable(n) {
		return protocol.NewError("runtime.not_focusable", "focus target is not interactive")
	}
	if s.FocusedID == id {
		return nil
	}
	s.FocusedID = id
	s.RenderGeneration++
	return nil
}

func (s *State) SetInputValue(id, value string) bool {
	if s.InputValues[id] == value {
		return false
	}
	s.InputValues[id] = value
	s.RenderGeneration++
	return true
}

func focusNeedsFallback(op protocol.Operation, eff document.Effect) bool {
	return op.Op == protocol.OpRemove || op.Op == protocol.OpProps || op.Op == protocol.OpUpsert || eff.TypeChanged
}

func firstFocusable(d document.Document, id string) string {
	n, ok := d.Nodes[id]
	if !ok {
		return ""
	}
	if id != "root" && focusable(n) {
		return id
	}
	for _, c := range n.Children {
		if found := firstFocusable(d, c); found != "" {
			return found
		}
	}
	return ""
}

// FocusableIDs returns renderer-independent focus traversal order in document
// tree preorder.
func FocusableIDs(d document.Document) []string {
	var out []string
	var walk func(string)
	walk = func(id string) {
		n, ok := d.Nodes[id]
		if !ok {
			return
		}
		if id != "root" && focusable(n) {
			out = append(out, id)
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk("root")
	return out
}
