package runtime

import (
	"fmt"
	"sync"

	"a2ui/protocol"
)

type EventClass int

const (
	EventCritical EventClass = iota
	EventCoalescible
	EventTelemetry
)

type EventBroker struct {
	mu        sync.Mutex
	cap       int
	nextOrder uint64
	nextSeq   uint64
	critical  []protocol.Event
	coalesced map[string]protocol.Event
	telemetry []protocol.Event
}

func NewEventBroker(capacity int) *EventBroker {
	if capacity < 1 {
		capacity = 1
	}
	return &EventBroker{cap: capacity, coalesced: map[string]protocol.Event{}}
}

// order stamps an internal enqueue order into Seq. Next overwrites it with the
// contiguous externally visible delivery sequence. The internal order is also
// exposed separately as a causal cursor for agent-facing publication waits.
func (b *EventBroker) order(ev protocol.Event) protocol.Event {
	b.nextOrder++
	ev.Seq = b.nextOrder
	return ev
}

// Cursor returns the latest internal enqueue position. It deliberately does not
// redefine protocol.Event.Seq, whose V1 contract remains contiguous delivery
// order even when coalescing replaces an undelivered event.
func (b *EventBroker) Cursor() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nextOrder
}

func (b *EventBroker) depthLocked() int {
	return len(b.critical) + len(b.coalesced) + len(b.telemetry)
}

func (b *EventBroker) Enqueue(ev protocol.Event, class EventClass) *protocol.Error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch class {
	case EventCritical:
		if b.depthLocked() >= b.cap {
			return protocol.NewError("runtime.backpressure_exceeded", fmt.Sprintf("pending event queue limit %d reached", b.cap))
		}
		b.critical = append(b.critical, b.order(ev))
	case EventCoalescible:
		key := ev.Ev + "|" + ev.ID
		if _, exists := b.coalesced[key]; !exists && b.depthLocked() >= b.cap {
			return protocol.NewError("runtime.backpressure_exceeded", fmt.Sprintf("pending event queue limit %d reached", b.cap))
		}
		b.coalesced[key] = b.order(ev)
	case EventTelemetry:
		if b.depthLocked() >= b.cap {
			if len(b.telemetry) == 0 {
				// Telemetry is explicitly lossy; never displace reliable/state events.
				return nil
			}
			b.telemetry = b.telemetry[1:]
		}
		b.telemetry = append(b.telemetry, b.order(ev))
	default:
		return protocol.NewError("runtime.invalid_event_class", fmt.Sprintf("unknown event class %d", class))
	}
	return nil
}

// EnqueueCriticalBatch appends a group of reliable events atomically. Either
// every event is accepted or none are, so publication acknowledgements cannot
// be partially emitted under backpressure.
func (b *EventBroker) EnqueueCriticalBatch(events []protocol.Event) *protocol.Error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(events) == 0 {
		return nil
	}
	if b.depthLocked()+len(events) > b.cap {
		return protocol.NewError("runtime.backpressure_exceeded", fmt.Sprintf("pending event queue limit %d reached", b.cap))
	}
	for _, ev := range events {
		b.critical = append(b.critical, b.order(ev))
	}
	return nil
}

func (b *EventBroker) deliver(ev protocol.Event) protocol.Event {
	b.nextSeq++
	ev.V = protocol.Version
	ev.Seq = b.nextSeq
	return ev
}

// NextWithCursor returns one event together with the immutable enqueue position
// at which that event entered the broker. The returned Event.Seq remains the V1
// contiguous delivery sequence.
func (b *EventBroker) NextWithCursor() (protocol.Event, uint64, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.critical) > 0 {
		ev := b.critical[0]
		b.critical = b.critical[1:]
		cursor := ev.Seq
		return b.deliver(ev), cursor, true
	}
	if len(b.coalesced) > 0 {
		var chosenKey string
		var chosen protocol.Event
		for k, ev := range b.coalesced {
			if chosenKey == "" || ev.Seq < chosen.Seq {
				chosenKey, chosen = k, ev
			}
		}
		delete(b.coalesced, chosenKey)
		cursor := chosen.Seq
		return b.deliver(chosen), cursor, true
	}
	if len(b.telemetry) > 0 {
		ev := b.telemetry[0]
		b.telemetry = b.telemetry[1:]
		cursor := ev.Seq
		return b.deliver(ev), cursor, true
	}
	return protocol.Event{}, 0, false
}

func (b *EventBroker) Next() (protocol.Event, bool) {
	ev, _, ok := b.NextWithCursor()
	return ev, ok
}

func (b *EventBroker) Depth() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.depthLocked()
}
