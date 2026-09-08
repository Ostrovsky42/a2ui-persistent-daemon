package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"a2ui/protocol"
	"a2ui/wire"
)

func TestHardwareFollowupProgressLabelDoesNotDuplicateDerivedPercentage(t *testing.T) {
	path := filepath.Join("..", "examples", "air-r0-renderer-smoke-followup.ndjson")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var env protocol.Envelope
		if perr := wire.StrictUnmarshal(scanner.Bytes(), &env); perr != nil {
			t.Fatal(perr)
		}
		var op protocol.Operation
		if perr := wire.StrictUnmarshal(env.Payload, &op); perr != nil {
			t.Fatal(perr)
		}
		if op.ID != "progress" || op.Op != protocol.OpProps {
			continue
		}
		var props struct {
			Label string `json:"label"`
		}
		if err := json.Unmarshal(op.Props, &props); err != nil {
			t.Fatal(err)
		}
		if props.Label != "detached agent update" {
			t.Fatalf("follow-up progress label=%q want renderer-neutral descriptive label", props.Label)
		}
		return
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Fatal("follow-up progress props not found")
}
