package session

import (
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	"testing"
)

func TestHelloNegotiatesVersionCapabilitiesAndLimits(t *testing.T) {
	s := New("s1", protocol.DefaultLimits())
	ack, err := s.Negotiate(protocol.Hello{Versions: []int{1}, Features: []string{"commit-barrier"}})
	if err != nil {
		t.Fatal(err)
	}
	if ack.Version != 1 || len(ack.Components) != 7 || ack.Limits.MaxNodes == 0 || s.State() != Ready {
		t.Fatalf("ack=%+v state=%v", ack, s.State())
	}
}

func TestHelloRejectsUnsupportedVersion(t *testing.T) {
	s := New("s1", protocol.DefaultLimits())
	_, err := s.Negotiate(protocol.Hello{Versions: []int{2}})
	if err == nil || err.Code != "protocol.version_mismatch" || err.Recoverable {
		t.Fatalf("got %#v", err)
	}
}

func TestReliableSequenceDuplicateAndGap(t *testing.T) {
	s := New("s", protocol.DefaultLimits())
	_, _ = s.Negotiate(protocol.Hello{Versions: []int{1}})
	dup, err := s.AcceptMutation(1)
	if err != nil || dup {
		t.Fatalf("first: dup=%v err=%v", dup, err)
	}
	dup, err = s.AcceptMutation(1)
	if err != nil || !dup {
		t.Fatalf("duplicate: dup=%v err=%v", dup, err)
	}
	_, err = s.AcceptMutation(3)
	if err == nil || err.Code != "protocol.sequence_gap" || err.Recoverable {
		t.Fatalf("gap=%#v", err)
	}
	if s.State() != Closed {
		t.Fatalf("fatal reliable-sequence gap must close the session, state=%v", s.State())
	}
	dup, err = s.AcceptMutation(2)
	if err == nil || dup || err.Code != "protocol.invalid_state" {
		t.Fatalf("closed session accepted seq2: dup=%v err=%v", dup, err)
	}
}

func TestReliableSequenceZeroIsFatal(t *testing.T) {
	s := New("s", protocol.DefaultLimits())
	_, _ = s.Negotiate(protocol.Hello{Versions: []int{1}})

	dup, err := s.AcceptMutation(0)
	if err == nil || err.Code != "protocol.invalid_sequence" || err.Recoverable || dup {
		t.Fatalf("seq0: dup=%v err=%#v", dup, err)
	}
	if s.State() != Closed {
		t.Fatalf("invalid reliable sequence must close the session, state=%v", s.State())
	}
}

func TestSessionAdvertisesOnlyFiniteEffectiveLimits(t *testing.T) {
	s := New("s", protocol.Limits{MaxNodes: 7})
	ack, err := s.Negotiate(protocol.Hello{Versions: []int{1}})
	if err != nil {
		t.Fatal(err)
	}
	if ack.Limits.MaxNodes != 7 {
		t.Fatalf("explicit limit lost: %+v", ack.Limits)
	}
	if perr := protocol.ValidateLimits(ack.Limits); perr != nil {
		t.Fatalf("session advertised invalid limits: %v", perr)
	}
}

func TestSessionRejectsSchemaInvalidHelloBeforeNegotiation(t *testing.T) {
	s := New("s", protocol.DefaultLimits())
	_, err := s.Negotiate(protocol.Hello{Versions: []int{1, 1}})
	if err == nil || err.Code != "schema.invalid_hello" {
		t.Fatalf("got %#v", err)
	}
	if s.State() != NewState {
		t.Fatalf("invalid hello changed session state: %v", s.State())
	}
}
