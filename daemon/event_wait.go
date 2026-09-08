package daemon

import (
	"context"
	"time"

	"a2ui/protocol"
)

// EventWaitRequest is an agent-facing causal filter over the existing reliable
// semantic event stream. AfterCursor is the broker enqueue position captured at
// publication time; it is intentionally distinct from protocol.Event.Seq.
type EventWaitRequest struct {
	AfterCursor uint64
	Frame       string
	Revision    uint64
	EventTypes  []string
}

type EventWaitResult struct {
	MatchedEvents  []protocol.Event
	ObservedEvents []protocol.Event
	TimedOut       bool
}

func eventMatchesWait(ev protocol.Event, cursor uint64, req EventWaitRequest, wanted map[string]struct{}) bool {
	if cursor <= req.AfterCursor {
		return false
	}
	if len(wanted) > 0 {
		if _, ok := wanted[ev.Ev]; !ok {
			return false
		}
	}
	if req.Frame != "" && ev.Frame != req.Frame {
		return false
	}
	if req.Revision != 0 && ev.Revision != req.Revision {
		return false
	}
	return true
}

// WaitSemanticEvents consumes the same Engine/EventBroker stream as legacy
// drains, but classifies events against an immutable publication cursor. Stale
// reliable events remain observable in ObservedEvents; they are never silently
// flushed merely because they cannot satisfy this waiter.
func (d *Daemon) WaitSemanticEvents(ctx context.Context, maxWait time.Duration, req EventWaitRequest) EventWaitResult {
	out := EventWaitResult{
		MatchedEvents:  []protocol.Event{},
		ObservedEvents: []protocol.Event{},
	}
	wanted := make(map[string]struct{}, len(req.EventTypes))
	for _, eventType := range req.EventTypes {
		wanted[eventType] = struct{}{}
	}

	drain := func() bool {
		for {
			ev, cursor, ok := d.Engine.NextEventWithCursor()
			if !ok {
				return len(out.MatchedEvents) > 0
			}
			out.ObservedEvents = append(out.ObservedEvents, ev)
			if eventMatchesWait(ev, cursor, req, wanted) {
				out.MatchedEvents = append(out.MatchedEvents, ev)
			}
		}
	}

	if drain() {
		return out
	}
	if maxWait <= 0 {
		out.TimedOut = true
		return out
	}

	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return out
		case <-timer.C:
			if !drain() {
				out.TimedOut = true
			}
			return out
		case <-d.eventCh:
			if drain() {
				return out
			}
		}
	}
}
