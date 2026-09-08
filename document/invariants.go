package document

import (
	"fmt"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func validateInvariants(d Document, limits protocol.Limits) *protocol.Error {
	root, ok := d.Nodes["root"]
	if !ok || root.Type != protocol.NodeBox || root.ID != "root" || root.Parent != "" {
		e := protocol.NewError("document.invariant", "root missing or invalid")
		e.Recoverable = false
		return e
	}
	if len(d.Nodes) > limits.MaxNodes {
		return protocol.NewError("resource.node_limit", fmt.Sprintf("node count %d exceeds %d", len(d.Nodes), limits.MaxNodes))
	}
	seen := map[string]bool{}
	visited := map[string]bool{}
	var walk func(string, int) *protocol.Error
	walk = func(id string, depth int) *protocol.Error {
		if depth > limits.MaxDepth {
			return protocol.NewError("resource.depth_limit", fmt.Sprintf("depth exceeds %d", limits.MaxDepth))
		}
		if seen[id] {
			e := protocol.NewError("document.cycle", "cycle detected")
			e.Recoverable = false
			return e
		}
		if visited[id] {
			e := protocol.NewError("document.invariant", fmt.Sprintf("node %q is referenced more than once", id))
			e.Recoverable = false
			return e
		}
		seen[id] = true
		visited[id] = true
		n, ok := d.Nodes[id]
		if !ok {
			e := protocol.NewError("document.invariant", "child missing")
			e.Recoverable = false
			return e
		}
		if n.ID != id || !n.Type.Valid() {
			e := protocol.NewError("document.invariant", fmt.Sprintf("node %q has invalid identity or type", id))
			e.Recoverable = false
			return e
		}
		if len(n.Children) > limits.MaxChildren {
			return protocol.NewError("resource.children_limit", fmt.Sprintf("node %q has too many children", id))
		}
		if len(n.Children) > 0 && !n.Type.Container() {
			return protocol.NewError("document.parent_not_container", fmt.Sprintf("node %q is not a container", id))
		}
		for _, cid := range n.Children {
			c, ok := d.Nodes[cid]
			if !ok || c.Parent != id {
				e := protocol.NewError("document.invariant", fmt.Sprintf("broken parent relation for %q", cid))
				e.Recoverable = false
				return e
			}
			if err := walk(cid, depth+1); err != nil {
				return err
			}
		}
		seen[id] = false
		return nil
	}
	if err := walk("root", 0); err != nil {
		return err
	}
	if len(visited) != len(d.Nodes) {
		e := protocol.NewError("document.invariant", "orphan nodes detected")
		e.Recoverable = false
		return e
	}
	actualTextBytes := 0
	documentBytes := 0
	for id, n := range d.Nodes {
		actualTextBytes += len(n.Text)
		documentBytes += len(id) + len(n.Parent) + len(n.Text)
		for _, child := range n.Children {
			documentBytes += len(child)
		}
		for key, value := range n.Props {
			documentBytes += len(key) + len(value)
		}
		for key, explicit := range n.ExplicitProps {
			if explicit {
				documentBytes += len(key) + 1
			}
		}
	}
	if documentBytes > limits.MaxDocumentBytes {
		return protocol.NewError("resource.document_bytes_limit", fmt.Sprintf("document bytes %d exceed %d", documentBytes, limits.MaxDocumentBytes))
	}
	if actualTextBytes != d.TotalTextBytes {
		e := protocol.NewError("document.invariant", fmt.Sprintf("retained text accounting mismatch: stored=%d actual=%d", d.TotalTextBytes, actualTextBytes))
		e.Recoverable = false
		return e
	}
	if d.TotalTextBytes > limits.MaxTotalTextBytes {
		return protocol.NewError("resource.text_too_large", "total retained text limit exceeded")
	}
	return nil
}

// Validate checks all structural and resource invariants of a document.
func Validate(d Document, limits protocol.Limits) *protocol.Error {
	return validateInvariants(d, limits)
}
