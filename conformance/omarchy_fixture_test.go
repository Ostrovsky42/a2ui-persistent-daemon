package conformance

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"a2ui/daemon"
	"a2ui/protocol"
	"a2ui/wire"
)

func TestOmarchyChoiceFixtureReplaysAllSixOperations(t *testing.T) {
	f, err := os.Open("../assets/examples/omarchy-choice.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	d := daemon.New("omarchy-choice", protocol.DefaultLimits(), nil)
	rd := wire.NewNDJSONReader(f, protocol.DefaultLimits().MaxMessageBytes)
	seen := map[protocol.OpType]bool{}

	for {
		env, perr, err := rd.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if perr != nil {
			t.Fatal(perr)
		}
		if env.Kind == protocol.KindOperation {
			var op protocol.Operation
			if err := json.Unmarshal(env.Payload, &op); err != nil {
				t.Fatal(err)
			}
			seen[op.Op] = true
		}
		if _, perr := d.HandleEnvelope(env); perr != nil {
			t.Fatalf("handle %s seq=%d: %v", env.Kind, env.Seq, perr)
		}
	}

	for _, op := range []protocol.OpType{
		protocol.OpUpsert,
		protocol.OpProps,
		protocol.OpText,
		protocol.OpRemove,
		protocol.OpFocus,
		protocol.OpCommit,
	} {
		if !seen[op] {
			t.Errorf("fixture never exercised %q", op)
		}
	}

	doc := d.Engine.Document()
	if _, ok := doc.Nodes["choice-panel"]; !ok {
		t.Fatal("fixture did not retain choice-panel")
	}
	if _, ok := doc.Nodes["temporary-note"]; ok {
		t.Fatal("fixture remove operation did not remove temporary-note")
	}
	if got := d.Engine.FocusedID(); got != "comment" {
		t.Fatalf("focused=%q, want comment", got)
	}
	if !d.Engine.NeedsPublish() {
		t.Fatal("commit fixture must leave publication pending before renderer acknowledgement")
	}
	if perr := d.Engine.Publish(); perr != nil {
		t.Fatal(perr)
	}
	ev, ok := d.Engine.NextEvent()
	if !ok || ev.Ev != "committed" || ev.Frame != "choice-ready" {
		t.Fatalf("unexpected commit event: %+v ok=%v", ev, ok)
	}
}
