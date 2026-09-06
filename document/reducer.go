package document

import (
	"encoding/json"
	"fmt"
	"reflect"

	"a2ui/protocol"
)

func Apply(current Document, op protocol.Operation, limits protocol.Limits) (Document, Effect, *protocol.Error) {
	eff := Effect{Frame: op.Frame, ThroughSeq: op.Seq}
	if err := protocol.ValidateOperation(op, limits); err != nil {
		err.Seq = op.Seq
		err.NodeID = op.ID
		return current, eff, err
	}
	if op.Op == protocol.OpCommit {
		eff.Commit = true
		return current, eff, nil
	}
	cand := current.Clone()
	var err *protocol.Error
	switch op.Op {
	case protocol.OpUpsert:
		err = applyUpsert(&cand, op, &eff, limits)
	case protocol.OpProps:
		err = applyProps(&cand, op, &eff, limits)
	case protocol.OpText:
		err = applyText(&cand, op, &eff, limits)
	case protocol.OpRemove:
		err = applyRemove(&cand, op, &eff)
	case protocol.OpFocus:
		if _, ok := cand.Nodes[op.ID]; !ok {
			err = protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", op.ID))
		}
	}
	if err != nil {
		err.Seq = op.Seq
		err.NodeID = op.ID
		return current, eff, err
	}
	if eff.Changed {
		if err = validateInvariants(cand, limits); err != nil {
			err.Seq = op.Seq
			err.NodeID = op.ID
			return current, Effect{}, err
		}
		cand.Revision = current.Revision + 1
	}
	return cand, eff, nil
}

func explicitPropKeys(raw json.RawMessage) map[string]bool {
	out := map[string]bool{}
	if len(raw) == 0 {
		return out
	}
	var props map[string]json.RawMessage
	if json.Unmarshal(raw, &props) != nil {
		return out
	}
	for key := range props {
		// force is a one-shot input mutation policy and never retained.
		if key != "force" {
			out[key] = true
		}
	}
	return out
}

func removeChild(children []string, id string) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		if child != id {
			out = append(out, child)
		}
	}
	return out
}

func insertChild(children []string, id string, index *int) []string {
	idx := len(children)
	if index != nil && *index >= 0 && *index < len(children) {
		idx = *index
	}
	children = append(children, "")
	copy(children[idx+1:], children[idx:])
	children[idx] = id
	return children
}

func wouldCreateCycle(d Document, id, targetParent string) bool {
	for current := targetParent; current != ""; {
		if current == id {
			return true
		}
		n, ok := d.Nodes[current]
		if !ok {
			return false
		}
		current = n.Parent
	}
	return false
}

func applyUpsert(d *Document, op protocol.Operation, eff *Effect, limits protocol.Limits) *protocol.Error {
	if op.ID == "root" {
		return protocol.NewError("document.root_immutable", "root cannot be upserted")
	}
	normalized, policy, err := protocol.NormalizeProps(op.Type, op.Props, true)
	if err != nil {
		return err
	}
	eff.Policy = policy
	explicit := explicitPropKeys(op.Props)
	n, exists := d.Nodes[op.ID]
	if exists {
		targetParentID := n.Parent
		if op.Parent != "" {
			targetParentID = op.Parent
		}
		parentChanged := targetParentID != n.Parent
		topologyRequested := parentChanged || op.Index != nil
		topologyChanged := false
		if topologyRequested {
			targetParent, ok := d.Nodes[targetParentID]
			if !ok {
				return protocol.NewError("document.parent_not_found", fmt.Sprintf("parent %q not found", targetParentID))
			}
			if wouldCreateCycle(*d, op.ID, targetParentID) {
				return protocol.NewError("document.cycle", fmt.Sprintf("node %q cannot move under %q", op.ID, targetParentID))
			}
			if !targetParent.Type.Container() {
				return protocol.NewError("document.parent_not_container", fmt.Sprintf("parent %q is %s", targetParentID, targetParent.Type))
			}

			oldParent := d.Nodes[n.Parent]
			if parentChanged {
				oldParent.Children = removeChild(oldParent.Children, op.ID)
				targetParent.Children = insertChild(targetParent.Children, op.ID, op.Index)
				d.Nodes[n.Parent] = oldParent
				d.Nodes[targetParentID] = targetParent
				n.Parent = targetParentID
				topologyChanged = true
			} else {
				reordered := insertChild(removeChild(oldParent.Children, op.ID), op.ID, op.Index)
				if !reflect.DeepEqual(oldParent.Children, reordered) {
					oldParent.Children = reordered
					d.Nodes[n.Parent] = oldParent
					topologyChanged = true
				}
			}
		}

		typeChanged := n.Type != op.Type
		if typeChanged {
			if len(n.Children) > 0 && !op.Type.Container() {
				return protocol.NewError("document.parent_not_container", "container with children cannot change to leaf")
			}
			d.TotalTextBytes -= len(n.Text)
			n.Type = op.Type
			n.Text = ""
			eff.TypeChanged = true
		}
		propsChanged := !reflect.DeepEqual(n.Props, normalized)
		explicitChanged := !reflect.DeepEqual(n.ExplicitProps, explicit)
		if propsChanged {
			n.Props = normalized
		}
		if explicitChanged {
			n.ExplicitProps = explicit
		}
		if err := validateTableLimits(n, limits); err != nil {
			return err
		}
		if topologyChanged || typeChanged || propsChanged || explicitChanged {
			d.Nodes[op.ID] = n
			eff.Changed = true
		}
		return nil
	}

	parentID := op.Parent
	if parentID == "" {
		parentID = "root"
	}
	p, ok := d.Nodes[parentID]
	if !ok {
		return protocol.NewError("document.parent_not_found", fmt.Sprintf("parent %q not found", parentID))
	}
	if !p.Type.Container() {
		return protocol.NewError("document.parent_not_container", fmt.Sprintf("parent %q is %s", parentID, p.Type))
	}
	n = Node{ID: op.ID, Type: op.Type, Parent: parentID, Props: normalized, ExplicitProps: explicit}
	d.Nodes[op.ID] = n
	p.Children = insertChild(p.Children, op.ID, op.Index)
	d.Nodes[parentID] = p
	eff.Changed = true
	return validateTableLimits(n, limits)
}

func validateTableLimits(n Node, limits protocol.Limits) *protocol.Error {
	if n.Type != protocol.NodeTable {
		return nil
	}
	var cols []json.RawMessage
	if raw := n.Props["columns"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &cols)
	}
	if len(cols) > limits.MaxTableColumns {
		return protocol.NewError("resource.table_columns_limit", "too many table columns")
	}
	var rows []json.RawMessage
	if raw := n.Props["rows"]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &rows)
	}
	if len(rows) > limits.MaxTableRows {
		return protocol.NewError("resource.table_rows_limit", "too many table rows")
	}
	return nil
}

func applyProps(d *Document, op protocol.Operation, eff *Effect, limits protocol.Limits) *protocol.Error {
	n, ok := d.Nodes[op.ID]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", op.ID))
	}
	merged, policy, err := protocol.MergeNormalized(n.Type, n.Props, op.Props)
	if err != nil {
		return err
	}
	eff.Policy = policy
	explicit := cloneExplicit(n.ExplicitProps)
	for key := range explicitPropKeys(op.Props) {
		explicit[key] = true
	}
	if reflect.DeepEqual(n.Props, merged) && reflect.DeepEqual(n.ExplicitProps, explicit) {
		return nil
	}
	n.Props = merged
	n.ExplicitProps = explicit
	if err := validateTableLimits(n, limits); err != nil {
		return err
	}
	d.Nodes[op.ID] = n
	eff.Changed = true
	return nil
}

func applyText(d *Document, op protocol.Operation, eff *Effect, limits protocol.Limits) *protocol.Error {
	n, ok := d.Nodes[op.ID]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", op.ID))
	}
	if n.Type != protocol.NodeText && n.Type != protocol.NodeViewport {
		return protocol.NewError("document.text_unsupported", fmt.Sprintf("node %q does not support text append", op.ID))
	}
	if op.Text == "" {
		return nil
	}
	if len(n.Text)+len(op.Text) > limits.MaxTextBytesPerNode {
		return protocol.NewError("resource.text_too_large", "node retained text limit exceeded")
	}
	if d.TotalTextBytes+len(op.Text) > limits.MaxTotalTextBytes {
		return protocol.NewError("resource.text_too_large", "total retained text limit exceeded")
	}
	n.Text += op.Text
	d.TotalTextBytes += len(op.Text)
	d.Nodes[op.ID] = n
	eff.Changed = true
	return nil
}

func applyRemove(d *Document, op protocol.Operation, eff *Effect) *protocol.Error {
	if op.ID == "root" {
		return protocol.NewError("document.root_immutable", "root cannot be removed")
	}
	n, ok := d.Nodes[op.ID]
	if !ok {
		return protocol.NewError("document.node_not_found", fmt.Sprintf("node %q not found", op.ID))
	}
	p := d.Nodes[n.Parent]
	for i, id := range p.Children {
		if id == op.ID {
			p.Children = append(p.Children[:i], p.Children[i+1:]...)
			break
		}
	}
	d.Nodes[n.Parent] = p
	var sweep func(string)
	sweep = func(id string) {
		n := d.Nodes[id]
		for _, c := range n.Children {
			sweep(c)
		}
		d.TotalTextBytes -= len(n.Text)
		delete(d.Nodes, id)
		eff.Removed = append(eff.Removed, id)
	}
	sweep(op.ID)
	eff.Changed = true
	return nil
}
