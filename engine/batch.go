package engine

import (
	"a2ui/document"
	"a2ui/protocol"
)

// ApplyBatch validates and reduces an agent-facing publication against candidate
// Document/Runtime state. Nothing becomes authoritative unless every operation
// succeeds. V1 operation and commit semantics are unchanged.
func (e *Engine) ApplyBatch(ops []protocol.Operation) *protocol.Error {
	e.mu.Lock()
	defer e.mu.Unlock()

	candidateDoc := e.doc.Clone()
	candidateState := e.state.Clone()
	candidateCommits := append([]commitRequest(nil), e.pendingCommits...)
	candidatePublicationGeneration := e.publicationGeneration
	pendingFrame := e.pendingFrame
	pendingRevision := e.pendingRevision

	for _, op := range ops {
		beforePublication := candidateState.PublicationGeneration
		next, eff, perr := document.Apply(candidateDoc, op, e.limits)
		if perr != nil {
			return perr
		}
		if eff.Commit && len(candidateCommits) >= e.maxPendingCommits {
			return protocol.NewError("runtime.backpressure_exceeded", "pending commit limit reached")
		}
		if rerr := candidateState.Reconcile(candidateDoc, next, op, eff); rerr != nil {
			return rerr
		}
		candidateDoc = next
		if eff.Commit {
			candidateCommits = append(candidateCommits, commitRequest{through: uint64(max64(op.Seq, 0)), frame: op.Frame})
			pendingFrame = op.Frame
			pendingRevision = candidateDoc.Revision
		}
		if candidateState.PublicationGeneration != beforePublication || eff.Commit {
			candidatePublicationGeneration++
		}
	}

	e.doc = candidateDoc
	e.state = candidateState
	e.pendingCommits = candidateCommits
	e.publicationGeneration = candidatePublicationGeneration
	e.pendingFrame = pendingFrame
	e.pendingRevision = pendingRevision
	return nil
}
