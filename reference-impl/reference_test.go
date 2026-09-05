package a2ui

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
)

func TestVerifiedReferenceFacadeUsesHardenedEngine(t *testing.T) {
	e := NewVerifiedEngine()
	if err := e.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "main", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{"gap":0}`)}); err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "main", Props: json.RawMessage(`{"gap":3}`)}); err != nil {
		t.Fatal(err)
	}
	var gap int
	_ = json.Unmarshal(e.Document().Nodes["main"].Props["gap"], &gap)
	if gap != 3 {
		t.Fatalf("gap=%d", gap)
	}
}
