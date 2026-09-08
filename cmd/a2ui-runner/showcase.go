package main

import (
	"encoding/json"
	"fmt"

	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

// buildShowcaseEngine creates one canonical semantic document used to inspect
// all renderer presets and responsive behavior. Preset selection is deliberately
// absent here: it belongs to the renderer, not the A2UI document.
func buildShowcaseEngine() (*engine.Engine, error) {
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

	if err := upsert("shell", protocol.NodeBox, "root", `{"variant":"section","dir":"col","gap":1}`); err != nil {
		return nil, err
	}
	if err := upsert("title", protocol.NodeText, "shell", `{"text":"A2UI Renderer V2","variant":"title"}`); err != nil {
		return nil, err
	}
	if err := upsert("subtitle", protocol.NodeText, "shell", `{"text":"One semantic document · adaptive presentation · renderer-local motion","variant":"subtitle"}`); err != nil {
		return nil, err
	}

	if err := upsert("cards", protocol.NodeBox, "shell", `{"dir":"row","responsive":"stack","gap":1}`); err != nil {
		return nil, err
	}
	if err := upsert("main-card", protocol.NodeBox, "cards", `{"variant":"card","dir":"col","gap":1,"flex":{"grow":2,"basis":36,"min_width":28}}`); err != nil {
		return nil, err
	}
	if err := upsert("main-label", protocol.NodeText, "main-card", `{"text":"AGENT SESSION","variant":"label"}`); err != nil {
		return nil, err
	}
	if err := upsert("main-body", protocol.NodeText, "main-card", `{"text":"Streaming tool output remains semantic while the renderer controls density, borders and responsive layout.","variant":"body"}`); err != nil {
		return nil, err
	}
	if err := upsert("loading", protocol.NodeProgress, "main-card", `{"variant":"spinner","state":"loading","label":"Synchronizing MCP tools"}`); err != nil {
		return nil, err
	}
	if err := upsert("download", protocol.NodeProgress, "main-card", `{"variant":"bar","state":"normal","value":0.72,"label":"Context hydrated"}`); err != nil {
		return nil, err
	}

	if err := upsert("status-card", protocol.NodeBox, "cards", `{"variant":"card","dir":"col","gap":1,"flex":{"grow":1,"basis":24,"min_width":22,"max_width":34}}`); err != nil {
		return nil, err
	}
	if err := upsert("status-title", protocol.NodeText, "status-card", `{"text":"RUNTIME","variant":"label"}`); err != nil {
		return nil, err
	}
	if err := upsert("ok", protocol.NodeProgress, "status-card", `{"variant":"compact","state":"success","value":1,"label":"Engine ready"}`); err != nil {
		return nil, err
	}
	if err := upsert("warning", protocol.NodeProgress, "status-card", `{"variant":"compact","state":"error","value":0,"label":"1 degraded probe"}`); err != nil {
		return nil, err
	}

	if err := upsert("table-title", protocol.NodeText, "shell", `{"text":"Workers","variant":"subtitle"}`); err != nil {
		return nil, err
	}
	if err := upsert("workers", protocol.NodeTable, "shell", `{
		"variant":"compact",
		"columns":[
			{"title":"Node","width":18},
			{"title":"Role","width":16},
			{"title":"Status","width":10},
			{"title":"Load","width":8}
		],
		"rows":[
			["local-core","orchestrator","READY","18%"],
			["mcp-bridge","transport","READY","7%"],
			["tool-worker-3","executor","BUSY","64%"]
		],
		"selectable":true
	}`); err != nil {
		return nil, err
	}

	if err := upsert("log-title", protocol.NodeText, "shell", `{"text":"Live log","variant":"subtitle"}`); err != nil {
		return nil, err
	}
	if err := upsert("log-view", protocol.NodeViewport, "shell", `{"height":4,"wrap":true,"follow_tail":true}`); err != nil {
		return nil, err
	}
	if err := upsert("log-text", protocol.NodeText, "log-view", `{"variant":"code","text":"04:13:02 session negotiated\n04:13:03 tools discovered\n04:13:04 document committed\n04:13:05 renderer preset resolved\n04:13:06 follow-tail active"}`); err != nil {
		return nil, err
	}

	if err := upsert("actions", protocol.NodeActions, "shell", `{"variant":"toolbar","items":[
		{"key":"r","label":"Refresh","action":"refresh"},
		{"key":"l","label":"Logs","action":"logs"},
		{"key":"q","label":"Quit","action":"quit"}
	]}`); err != nil {
		return nil, err
	}

	if perr := eng.Focus("workers"); perr != nil {
		return nil, fmt.Errorf("focus workers: %s", perr)
	}
	if perr := eng.Apply(protocol.Operation{V: protocol.Version, Seq: seq, Op: protocol.OpCommit, Frame: "renderer-v2-showcase"}); perr != nil {
		return nil, fmt.Errorf("commit showcase: %s", perr)
	}
	return eng, nil
}
