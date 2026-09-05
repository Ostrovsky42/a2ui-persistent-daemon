package main

import (
	"encoding/json"
	"testing"

	"a2ui/protocol"
)

func TestBuildInteractiveEngineCreatesV3InteractionSurface(t *testing.T) {
	eng, err := buildInteractiveEngine()
	if err != nil {
		t.Fatal(err)
	}
	doc := eng.Document()
	jobs := doc.Nodes["jobs"]
	logs := doc.Nodes["logs"]
	command := doc.Nodes["command"]
	if jobs.Type != protocol.NodeTable || logs.Type != protocol.NodeViewport || command.Type != protocol.NodeInput {
		t.Fatalf("interactive scenario missing table/viewport/input: jobs=%s logs=%s command=%s", jobs.Type, logs.Type, command.Type)
	}
	var scrollable bool
	if err := json.Unmarshal(logs.Props["scrollable"], &scrollable); err != nil || !scrollable {
		t.Fatalf("logs viewport must be scrollable, raw=%s err=%v", logs.Props["scrollable"], err)
	}
	var rowIDs []string
	if err := json.Unmarshal(jobs.Props["row_ids"], &rowIDs); err != nil || len(rowIDs) != 3 {
		t.Fatalf("jobs table must expose stable row IDs, row_ids=%v err=%v", rowIDs, err)
	}
	if eng.FocusedID() != "jobs" {
		t.Fatalf("expected initial focus on jobs, got %q", eng.FocusedID())
	}
}
