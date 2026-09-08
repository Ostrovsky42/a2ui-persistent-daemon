package document

import (
	"encoding/json"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func TestValidateRejectsDuplicateChildReference(t *testing.T) {
	d := New()
	d.Nodes["child"] = Node{ID: "child", Type: protocol.NodeText, Parent: "root", Props: map[string]json.RawMessage{}}
	root := d.Nodes["root"]
	root.Children = []string{"child", "child"}
	d.Nodes["root"] = root

	err := Validate(d, protocol.DefaultLimits())
	if err == nil || err.Code != "document.invariant" || err.Recoverable {
		t.Fatalf("got %#v", err)
	}
}

func TestValidateRejectsNodeMapKeyIdentityMismatch(t *testing.T) {
	d := New()
	d.Nodes["child"] = Node{ID: "other", Type: protocol.NodeText, Parent: "root", Props: map[string]json.RawMessage{}}
	root := d.Nodes["root"]
	root.Children = []string{"child"}
	d.Nodes["root"] = root

	err := Validate(d, protocol.DefaultLimits())
	if err == nil || err.Code != "document.invariant" || err.Recoverable {
		t.Fatalf("got %#v", err)
	}
}

func TestValidateRejectsRootParent(t *testing.T) {
	d := New()
	root := d.Nodes["root"]
	root.Parent = "somewhere"
	d.Nodes["root"] = root

	err := Validate(d, protocol.DefaultLimits())
	if err == nil || err.Code != "document.invariant" || err.Recoverable {
		t.Fatalf("got %#v", err)
	}
}

func TestValidateRejectsUnknownNodeType(t *testing.T) {
	d := New()
	d.Nodes["child"] = Node{ID: "child", Type: protocol.NodeType("alien"), Parent: "root", Props: map[string]json.RawMessage{}}
	root := d.Nodes["root"]
	root.Children = []string{"child"}
	d.Nodes["root"] = root

	err := Validate(d, protocol.DefaultLimits())
	if err == nil || err.Code != "document.invariant" || err.Recoverable {
		t.Fatalf("got %#v", err)
	}
}
