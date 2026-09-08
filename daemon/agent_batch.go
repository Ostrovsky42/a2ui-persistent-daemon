package daemon

import (
	"context"
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

	seqs := make([]uint64, len(batch.Operations))
	for i, op := range batch.Operations {
		if op.Seq <= 0 {
			return nil, protocol.NewError("protocol.invalid_sequence", "reliable mutation seq must start at 1")
		}
		seqs[i] = uint64(op.Seq)
	}
	plan, perr := d.Session.PrepareMutationBatch(seqs)
	if perr != nil {
		return nil, perr
	}

	toApply := make([]protocol.Operation, 0, len(batch.Operations))
	for i, op := range batch.Operations {
		if plan.Apply[i] {
			toApply = append(toApply, op)
		}
	}
	if perr := d.Engine.ApplyBatch(toApply); perr != nil {
		return nil, perr
	}
	if perr := d.Session.CommitMutationBatch(plan); perr != nil {
		return nil, perr
	}

	generation, _ := d.Engine.PublicationGeneration()
	frame, revision, _ := d.Engine.PublicationMetadata(generation)
	eventCursor := d.Engine.EventCursor()
	visible := false
	if len(toApply) > 0 {
		// One successful agent-facing batch creates at most one renderer update.
		d.signalSnapshot()
		visible = d.waitPublication(context.Background(), generation)
	}
	result, err := json.Marshal(map[string]any{"published": len(toApply), "frame": frame, "revision": revision, "publication_generation": generation, "event_cursor": eventCursor, "visible": visible})
	if err != nil {
		return nil, protocol.NewError("mcp.encode_failed", err.Error())
	}
	return &mcp.Message{JSONRPC: "2.0", ID: msg.ID, Result: result}, nil
}
