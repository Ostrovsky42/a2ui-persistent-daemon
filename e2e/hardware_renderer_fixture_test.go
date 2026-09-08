package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"a2ui/engine"
	"a2ui/protocol"
	"a2ui/wire"
)

func applyHardwareFixture(t *testing.T, eng *engine.Engine, name string, expectedSeq *uint64) {
	t.Helper()
	path := filepath.Join("..", "examples", name)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var env protocol.Envelope
		if perr := wire.StrictUnmarshal(line, &env); perr != nil {
			t.Fatalf("%s envelope: %v", name, perr)
		}
		*expectedSeq++
		if env.Session != "hardware-smoke" || env.Kind != protocol.KindOperation || env.Seq != *expectedSeq {
			t.Fatalf("%s envelope seq=%d session=%q kind=%q want seq=%d hardware-smoke/operation", name, env.Seq, env.Session, env.Kind, *expectedSeq)
		}
		var op protocol.Operation
		if perr := wire.StrictUnmarshal(env.Payload, &op); perr != nil {
			t.Fatalf("%s operation seq=%d: %v", name, env.Seq, perr)
		}
		if op.V != protocol.Version || op.Seq != int64(env.Seq) {
			t.Fatalf("%s operation envelope seq=%d op seq=%d v=%d", name, env.Seq, op.Seq, op.V)
		}
		if perr := eng.Apply(op); perr != nil {
			t.Fatalf("%s apply seq=%d: %v", name, env.Seq, perr)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
}

func propString(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestHardwareRendererFixturesPreserveRuntimeAcrossRendererSwitch(t *testing.T) {
	eng := engine.New(protocol.DefaultLimits(), 32, engine.NewNoopActions())
	var seq uint64
	applyHardwareFixture(t, eng, "air-r0-renderer-smoke-initial.ndjson", &seq)

	initial := eng.PresentationSnapshot()
	if got := propString(t, initial.Document.Nodes["title"].Props["text"]); got != "AIR R0 hardware smoke — TUI phase" {
		t.Fatalf("initial title=%q", got)
	}
	if !initial.PublicationPending {
		t.Fatal("initial fixture did not create a publication barrier")
	}
	if perr := eng.SetInput("answer", "from-tui"); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.SetTableSelectionByRowID("surfaces", "tui"); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.Publish(); perr != nil {
		t.Fatal(perr)
	}

	applyHardwareFixture(t, eng, "air-r0-renderer-smoke-followup.ndjson", &seq)
	followup := eng.PresentationSnapshot()
	if got := propString(t, followup.Document.Nodes["title"].Props["text"]); got != "AIR R0 hardware smoke — Web phase" {
		t.Fatalf("follow-up title=%q", got)
	}
	if got := followup.InputValues["answer"]; got != "from-tui" {
		t.Fatalf("renderer-local input state was lost across agent follow-up: %q", got)
	}
	selection := followup.TableSelections["surfaces"]
	if selection.RowID != "tui" || selection.Index != 1 {
		t.Fatalf("stable row selection after reorder=%+v", selection)
	}
	if !followup.PublicationPending {
		t.Fatal("follow-up fixture did not create a Web-phase publication barrier")
	}
	if seq != 10 {
		t.Fatalf("final sequence=%d want 10", seq)
	}
}
