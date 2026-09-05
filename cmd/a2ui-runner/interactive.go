package main

import (
	"encoding/json"
	"fmt"

	"a2ui/engine"
	"a2ui/protocol"
)

// buildInteractiveEngine creates the canonical V3 interaction surface. The
// document carries semantic capabilities while focus/selection live in runtime
// and caret/scroll mechanics remain Bubble Tea-local.
func buildInteractiveEngine() (*engine.Engine, error) {
	eng := engine.New(protocol.DefaultLimits(), 64, engine.NewNoopActions())
	seq := int64(1)
	upsert := func(id string, typ protocol.NodeType, parent, props string) error {
		op := protocol.Operation{V: protocol.Version, Seq: seq, Op: protocol.OpUpsert, ID: id, Type: typ, Parent: parent, Props: json.RawMessage(props)}
		seq++
		if perr := eng.Apply(op); perr != nil {
			return fmt.Errorf("upsert %s: %s", id, perr)
		}
		return nil
	}

	steps := []struct {
		id, parent, props string
		typ               protocol.NodeType
	}{
		{"shell", "root", `{"variant":"section","dir":"col","gap":1}`, protocol.NodeBox},
		{"title", "shell", `{"text":"A2UI V3 Interactive Runtime","variant":"title"}`, protocol.NodeText},
		{"help", "shell", `{"text":"Tab/Shift+Tab focus · ↑↓ navigate/scroll · PgUp/PgDn viewport · Home/End boundary · Enter activate/submit · Ctrl+C quit","variant":"muted"}`, protocol.NodeText},
		{"main", "shell", `{"dir":"row","responsive":"stack","gap":1}`, protocol.NodeBox},
		{"jobs", "main", `{"variant":"compact","selectable":true,"action":"open_job","row_ids":["job-a","job-b","job-c"],"columns":[{"title":"Job","width":18},{"title":"State","width":12}],"rows":[["job-a","queued"],["job-b","done"],["job-c","running"]],"flex":{"grow":1,"basis":32,"min_width":24}}`, protocol.NodeTable},
		{"details-card", "main", `{"variant":"card","dir":"col","gap":1,"flex":{"grow":1,"basis":32,"min_width":24}}`, protocol.NodeBox},
		{"details-title", "details-card", `{"text":"JOB DETAILS","variant":"label"}`, protocol.NodeText},
		{"details", "details-card", `{"text":"Select a job and press Enter","variant":"body"}`, protocol.NodeText},
		{"progress", "details-card", `{"variant":"compact","state":"loading","label":"Agent connection active"}`, protocol.NodeProgress},
		{"log-title", "shell", `{"text":"Live logs","variant":"subtitle"}`, protocol.NodeText},
		{"logs", "shell", `{"height":5,"wrap":true,"follow_tail":true,"scrollable":true}`, protocol.NodeViewport},
		{"log-text", "logs", `{"variant":"code","text":"L1 session ready\nL2 MCP negotiated\nL3 document projected\nL4 table selectable\nL5 viewport scrollable\nL6 follow-tail active\nL7 waiting for operator"}`, protocol.NodeText},
		{"command", "shell", `{"value":"","placeholder":"Operator command…","action":"run_command"}`, protocol.NodeInput},
		{"actions", "shell", `{"variant":"toolbar","items":[{"key":"r","label":"Restart","action":"restart"},{"key":"l","label":"Logs","action":"logs"}]}`, protocol.NodeActions},
	}
	for _, step := range steps {
		if err := upsert(step.id, step.typ, step.parent, step.props); err != nil {
			return nil, err
		}
	}
	if perr := eng.Focus("jobs"); perr != nil {
		return nil, fmt.Errorf("focus jobs: %s", perr)
	}
	if perr := eng.Apply(protocol.Operation{V: protocol.Version, Seq: seq, Op: protocol.OpCommit, Frame: "interactive-v3-initial"}); perr != nil {
		return nil, fmt.Errorf("commit interactive scenario: %s", perr)
	}
	return eng, nil
}
