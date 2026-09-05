package runtime

import (
	"encoding/json"
	"fmt"

	"a2ui/document"
	"a2ui/protocol"
)

type actionItem struct {
	Key    string          `json:"key"`
	Label  string          `json:"label"`
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args,omitempty"`
}

func buildBindings(d document.Document) (map[string]Binding, *protocol.Error) {
	out := map[string]Binding{}
	var walk func(string) *protocol.Error
	walk = func(id string) *protocol.Error {
		n := d.Nodes[id]
		if n.Type == protocol.NodeActions {
			var items []actionItem
			if raw := n.Props["items"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &items); err != nil {
					return protocol.NewError("runtime.invalid_actions", err.Error())
				}
			}
			for _, it := range items {
				if old, ok := out[it.Key]; ok {
					return nilOrConflict(it.Key, old.NodeID, id)
				}
				out[it.Key] = Binding{NodeID: id, Key: it.Key, Label: it.Label, Action: it.Action, Args: append(json.RawMessage(nil), it.Args...)}
			}
		}
		for _, c := range n.Children {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk("root"); err != nil {
		return nil, err
	}
	return out, nil
}

func nilOrConflict(key, a, b string) *protocol.Error {
	return protocol.NewError("runtime.binding_conflict", fmt.Sprintf("key %q is bound by both %q and %q", key, a, b))
}
