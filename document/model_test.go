package document

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func TestModelSequenceNeverBreaksDocumentInvariants(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	limits := protocol.DefaultLimits()
	d := New()
	for seq := int64(1); seq <= 2000; seq++ {
		id := fmt.Sprintf("n%d", r.Intn(24))
		var op protocol.Operation
		switch r.Intn(5) {
		case 0:
			typ := protocol.NodeText
			props := json.RawMessage(`{"text":"x"}`)
			if r.Intn(4) == 0 {
				typ = protocol.NodeBox
				props = json.RawMessage(fmt.Sprintf(`{"gap":%d}`, r.Intn(4)))
			}
			parent := "root"
			if p, ok := d.Nodes[fmt.Sprintf("n%d", r.Intn(24))]; ok && p.Type == protocol.NodeBox {
				parent = p.ID
			}
			op = protocol.Operation{V: 1, Seq: seq, Op: protocol.OpUpsert, ID: id, Type: typ, Parent: parent, Props: props}
		case 1:
			op = protocol.Operation{V: 1, Seq: seq, Op: protocol.OpProps, ID: id, Props: json.RawMessage(`{"style":{"fg":"primary"}}`)}
		case 2:
			op = protocol.Operation{V: 1, Seq: seq, Op: protocol.OpText, ID: id, Text: "x"}
		case 3:
			op = protocol.Operation{V: 1, Seq: seq, Op: protocol.OpRemove, ID: id}
		default:
			op = protocol.Operation{V: 1, Seq: seq, Op: protocol.OpCommit}
		}
		next, _, err := Apply(d, op, limits)
		if err == nil {
			d = next
		}
		if inv := Validate(d, limits); inv != nil {
			t.Fatalf("seq=%d op=%+v invariant=%v", seq, op, inv)
		}
	}
}

func FuzzApplyNeverBreaksDocumentInvariants(f *testing.F) {
	f.Add([]byte(`{"v":1,"seq":1,"op":"upsert","id":"x","type":"text","props":{"text":"ok"}}`))
	f.Add([]byte(`{"v":1,"seq":2,"op":"remove","id":"root"}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		var op protocol.Operation
		if json.Unmarshal(b, &op) != nil {
			return
		}
		d := New()
		next, _, err := Apply(d, op, protocol.DefaultLimits())
		if err == nil {
			d = next
		}
		if inv := Validate(d, protocol.DefaultLimits()); inv != nil {
			t.Fatalf("accepted/rejected operation left invalid document: %v", inv)
		}
	})
}
