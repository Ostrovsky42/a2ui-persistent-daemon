package daemon

import (
	"encoding/json"

	"a2ui/protocol"
	"a2ui/transport/mcp"
	"a2ui/wire"
)

type agentPublishBatch struct {
	Session    string               `json:"session"`
	Operations []protocol.Operation `json:"operations"`
}

// handleAgentPublishBatch is an ergonomic agent-facing transport boundary. It
// does not add a V1 mutation or redefine commit: the contained operations are
// still the six frozen A2UI V1 mutations, applied in their original order.
// P0.2 initially centralizes renderer signaling here; transaction rollback is
// hardened separately by the atomic-failure gate.
func (d *Daemon) handleAgentPublishBatch(msg mcp.Message) (*mcp.Message, *protocol.Error) {
	var batch agentPublishBatch
	if perr := wire.StrictUnmarshal(msg.Params, &batch); perr != nil {
		return nil, perr
	}
	if batch.Session != "" && batch.Session != d.Session.ID() {
		return nil, protocol.NewError("protocol.session_mismatch", "publish batch session does not match daemon session")
	}
	if len(batch.Operations) == 0 {
		return nil, protocol.NewError("protocol.empty_batch", "publish batch must contain at least one operation")
	}

	d.agentMu.Lock()
	defer d.agentMu.Unlock()

	published := 0
	for _, op := range batch.Operations {
		if op.Seq <= 0 {
			return nil, protocol.NewError("protocol.invalid_sequence", "reliable mutation seq must start at 1")
		}
		duplicate, perr := d.Session.AcceptMutation(uint64(op.Seq))
		if perr != nil {
			return nil, perr
		}
		if duplicate {
			continue
		}
		if perr := d.Engine.Apply(op); perr != nil {
			return nil, perr
		}
		published++
	}

	// The renderer sees one notification for the complete agent-facing batch.
	d.signalSnapshot()
	result, err := json.Marshal(map[string]any{"published": published})
	if err != nil {
		return nil, protocol.NewError("mcp.encode_failed", err.Error())
	}
	return &mcp.Message{JSONRPC: "2.0", ID: msg.ID, Result: result}, nil
}
