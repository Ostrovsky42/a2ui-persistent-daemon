package session

import (
	"fmt"

	"a2ui/protocol"
)

// MutationBatchPlan is an internal transaction plan for a contiguous agent
// publication. Preparing it validates duplicate/gap semantics without advancing
// the reliable mutation sequence. CommitMutationBatch advances only after the
// Engine candidate has succeeded.
type MutationBatchPlan struct {
	ExpectedBefore uint64
	NextExpected   uint64
	Apply          []bool
}

func (s *Session) PrepareMutationBatch(seqs []uint64) (MutationBatchPlan, *protocol.Error) {
	plan := MutationBatchPlan{
		ExpectedBefore: s.expectedMutation,
		NextExpected:   s.expectedMutation,
		Apply:          make([]bool, len(seqs)),
	}
	if s.state != Ready {
		return plan, protocol.NewError("protocol.invalid_state", "session is not READY")
	}

	expected := s.expectedMutation
	for i, seq := range seqs {
		if seq == 0 {
			e := protocol.NewError("protocol.invalid_sequence", "reliable mutation seq must start at 1")
			e.Recoverable = false
			s.state = Closed
			return plan, e
		}
		if seq < expected {
			continue
		}
		if seq > expected {
			e := protocol.NewError("protocol.sequence_gap", fmt.Sprintf("expected mutation seq %d, got %d", expected, seq))
			e.Recoverable = false
			s.state = Closed
			return plan, e
		}
		plan.Apply[i] = true
		expected++
	}
	plan.NextExpected = expected
	return plan, nil
}

func (s *Session) CommitMutationBatch(plan MutationBatchPlan) *protocol.Error {
	if s.state != Ready {
		return protocol.NewError("protocol.invalid_state", "session is not READY")
	}
	if s.expectedMutation != plan.ExpectedBefore {
		return protocol.NewError("protocol.invalid_state", "mutation batch plan is stale")
	}
	s.expectedMutation = plan.NextExpected
	return nil
}
