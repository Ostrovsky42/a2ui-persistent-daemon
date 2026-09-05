package session

import (
	"fmt"
	"sort"

	"a2ui/protocol"
)

type State int

const (
	NewState State = iota
	Ready
	Draining
	Closed
)

type Session struct {
	id               string
	state            State
	limits           protocol.Limits
	version          int
	expectedMutation uint64
	features         []string
}

func New(id string, limits protocol.Limits) *Session {
	limits = protocol.EffectiveLimits(limits)
	return &Session{id: id, state: NewState, limits: limits, expectedMutation: 1}
}
func (s *Session) State() State            { return s.state }
func (s *Session) ID() string              { return s.id }
func (s *Session) Limits() protocol.Limits { return s.limits }

func (s *Session) Negotiate(h protocol.Hello) (protocol.HelloAck, *protocol.Error) {
	if s.state != NewState {
		return protocol.HelloAck{}, protocol.NewError("protocol.invalid_state", "hello only allowed in NEW")
	}
	if err := protocol.ValidateHello(h); err != nil {
		return protocol.HelloAck{}, err
	}
	supported := false
	for _, v := range h.Versions {
		if v == protocol.Version {
			supported = true
			break
		}
	}
	if !supported {
		e := protocol.NewError("protocol.version_mismatch", "no mutually supported protocol version")
		e.Recoverable = false
		return protocol.HelloAck{}, e
	}
	available := map[string]bool{"commit-barrier": true, "incremental-props": true, "action-events": true, "bounded-resources": true, "udp-telemetry": true}
	features := make([]string, 0, len(h.Features))
	for _, f := range h.Features {
		if available[f] {
			features = append(features, f)
		}
	}
	sort.Strings(features)
	s.version = protocol.Version
	s.features = features
	s.state = Ready
	return protocol.HelloAck{Version: protocol.Version, Features: append([]string(nil), features...), Components: protocol.AllNodeTypes(), Limits: s.limits}, nil
}

func (s *Session) AcceptMutation(seq uint64) (bool, *protocol.Error) {
	if s.state != Ready {
		return false, protocol.NewError("protocol.invalid_state", "session is not READY")
	}
	if seq == 0 {
		e := protocol.NewError("protocol.invalid_sequence", "reliable mutation seq must start at 1")
		e.Recoverable = false
		s.state = Closed
		return false, e
	}
	if seq < s.expectedMutation {
		return true, nil
	}
	if seq > s.expectedMutation {
		e := protocol.NewError("protocol.sequence_gap", fmt.Sprintf("expected mutation seq %d, got %d", s.expectedMutation, seq))
		e.Recoverable = false
		s.state = Closed
		return false, e
	}
	s.expectedMutation++
	return false, nil
}

func (s *Session) Drain() {
	if s.state == Ready {
		s.state = Draining
	}
}
func (s *Session) Close() { s.state = Closed }
