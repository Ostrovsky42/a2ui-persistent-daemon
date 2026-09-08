package document

import (
	"encoding/json"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

type Node struct {
	ID            string
	Type          protocol.NodeType
	Parent        string
	Children      []string
	Props         map[string]json.RawMessage
	ExplicitProps map[string]bool
	Text          string
}

type Document struct {
	Revision       uint64
	Nodes          map[string]Node
	TotalTextBytes int
}

type Effect struct {
	Changed     bool
	Commit      bool
	Frame       string
	ThroughSeq  int64
	Removed     []string
	TypeChanged bool
	Policy      protocol.MutationPolicy
}

func New() Document {
	rootProps, _, _ := protocol.NormalizeProps(protocol.NodeBox, json.RawMessage(`{}`), true)
	return Document{Nodes: map[string]Node{"root": {ID: "root", Type: protocol.NodeBox, Props: rootProps}}}
}

func cloneRaw(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

func cloneExplicit(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (d Document) Clone() Document {
	out := Document{Revision: d.Revision, TotalTextBytes: d.TotalTextBytes, Nodes: make(map[string]Node, len(d.Nodes))}
	for id, n := range d.Nodes {
		n.Children = append([]string(nil), n.Children...)
		n.Props = cloneRaw(n.Props)
		n.ExplicitProps = cloneExplicit(n.ExplicitProps)
		out.Nodes[id] = n
	}
	return out
}
