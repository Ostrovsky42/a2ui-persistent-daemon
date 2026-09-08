package runtime

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"a2ui/protocol"
)

func TestCriticalEventsNeverSilentlyDrop(t *testing.T) {
	b := NewEventBroker(1)
	if err := b.Enqueue(protocol.Event{Ev: "submit", ID: "a"}, EventCritical); err != nil {
		t.Fatal(err)
	}
	err := b.Enqueue(protocol.Event{Ev: "action", ID: "b"}, EventCritical)
	if err == nil || err.Code != "runtime.backpressure_exceeded" {
		t.Fatalf("expected backpressure, got %#v", err)
	}
	ev, ok := b.Next()
	if !ok || ev.ID != "a" {
		t.Fatalf("queue corrupted: %+v %v", ev, ok)
	}
}

func TestResizeCoalescesToLatest(t *testing.T) {
	b := NewEventBroker(1)
	if err := b.Enqueue(protocol.Event{Ev: "resize", W: 80, H: 24}, EventCoalescible); err != nil {
		t.Fatal(err)
	}
	if err := b.Enqueue(protocol.Event{Ev: "resize", W: 120, H: 40}, EventCoalescible); err != nil {
		t.Fatal(err)
	}
	ev, ok := b.Next()
	if !ok || ev.W != 120 || ev.H != 40 {
		t.Fatalf("got %+v", ev)
	}
	if _, ok := b.Next(); ok {
		t.Fatal("coalesced resize produced duplicate event")
	}
}

func TestActionRegistryRejectsNilAndUnknown(t *testing.T) {
	r := NewActionRegistry(50*time.Millisecond, 2)
	if err := r.Register("bad", nil); err == nil {
		t.Fatal("nil handler accepted")
	}
	ev := r.Dispatch(context.Background(), "node", "missing", json.RawMessage(`{}`))
	if ev.Ev != "error" || ev.Code != "action.not_permitted" {
		t.Fatalf("got %+v", ev)
	}
}

func TestActionRegistryRejectsDuplicateRegistration(t *testing.T) {
	r := NewActionRegistry(time.Second, 1)
	if err := r.Register("delete", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("delete", func(context.Context, json.RawMessage) error { return nil }); err == nil {
		t.Fatal("duplicate action registration silently replaced the trusted handler")
	}
}

func TestActionTimeoutIsObservableAndDoesNotBlockCallerForever(t *testing.T) {
	r := NewActionRegistry(20*time.Millisecond, 1)
	if err := r.Register("slow", func(ctx context.Context, args json.RawMessage) error { <-ctx.Done(); return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	ev := r.Dispatch(context.Background(), "node", "slow", nil)
	if time.Since(start) > 200*time.Millisecond {
		t.Fatal("dispatch ignored timeout")
	}
	if ev.Ev != "error" || ev.Code != "action.timeout" || ev.InvocationID == "" {
		t.Fatalf("got %+v", ev)
	}
}

func TestActionSuccessHasInvocationID(t *testing.T) {
	r := NewActionRegistry(time.Second, 1)
	_ = r.Register("ok", func(context.Context, json.RawMessage) error { return nil })
	ev := r.Dispatch(context.Background(), "n", "ok", json.RawMessage(`{"x":1}`))
	if ev.Ev != "action_result" || ev.Action != "ok" || ev.InvocationID == "" {
		t.Fatalf("got %+v", ev)
	}
}

func TestNilActionRegistryDispatchIsSafe(t *testing.T) {
	var r *ActionRegistry
	ev := r.Dispatch(context.Background(), "n", "x", nil)
	if ev.Ev != "error" || ev.Code != "action.not_permitted" {
		t.Fatalf("got %+v", ev)
	}
}

func TestActionRegistryKeysAreSorted(t *testing.T) {
	r := NewActionRegistry(time.Second, 1)
	_ = r.Register("z", func(context.Context, json.RawMessage) error { return nil })
	_ = r.Register("a", func(context.Context, json.RawMessage) error { return nil })
	keys := r.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "z" {
		t.Fatalf("keys=%v", keys)
	}
}

func TestEventBrokerBoundsAllPendingClasses(t *testing.T) {
	b := NewEventBroker(1)
	if err := b.Enqueue(protocol.Event{Ev: "focus", ID: "a"}, EventCoalescible); err != nil {
		t.Fatal(err)
	}
	if err := b.Enqueue(protocol.Event{Ev: "focus", ID: "b"}, EventCoalescible); err == nil || err.Code != "runtime.backpressure_exceeded" {
		t.Fatalf("second distinct coalescible event must hit total capacity, got %#v", err)
	}
	if b.Depth() != 1 {
		t.Fatalf("depth=%d", b.Depth())
	}
}

func TestEventBrokerDeliveredSequenceStaysContiguousAfterCoalescing(t *testing.T) {
	b := NewEventBroker(4)
	if err := b.Enqueue(protocol.Event{Ev: "resize", W: 80, H: 24}, EventCoalescible); err != nil {
		t.Fatal(err)
	}
	if err := b.Enqueue(protocol.Event{Ev: "resize", W: 120, H: 40}, EventCoalescible); err != nil {
		t.Fatal(err)
	}
	if err := b.Enqueue(protocol.Event{Ev: "submit", ID: "x"}, EventCritical); err != nil {
		t.Fatal(err)
	}
	first, ok := b.Next()
	if !ok {
		t.Fatal("missing first event")
	}
	second, ok := b.Next()
	if !ok {
		t.Fatal("missing second event")
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("delivered sequence has a gap: first=%d second=%d", first.Seq, second.Seq)
	}
}

func TestEventBrokerPendingEventsDoNotUseProtocolSeqForQueueOrder(t *testing.T) {
	b := NewEventBroker(4)
	if err := b.Enqueue(protocol.Event{Ev: "resize", ID: "screen", W: 80, H: 24}, EventCoalescible); err != nil {
		t.Fatal(err)
	}
	if err := b.Enqueue(protocol.Event{Ev: "submit", ID: "input"}, EventCritical); err != nil {
		t.Fatal(err)
	}

	eventType := reflect.TypeOf(protocol.Event{})
	var stamped []uint64
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if !v.IsValid() {
			return
		}
		if v.Type() == eventType {
			seq := v.FieldByName("Seq").Uint()
			if seq != 0 {
				stamped = append(stamped, seq)
			}
			return
		}
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Map:
			iter := v.MapRange()
			for iter.Next() {
				walk(iter.Value())
			}
		}
	}
	walk(reflect.ValueOf(b).Elem())
	if len(stamped) != 0 {
		t.Fatalf("pending protocol events already carry delivery seq values: %v", stamped)
	}
}

func TestTimedOutHandlerKeepsInflightSlotUntilItActuallyReturns(t *testing.T) {
	r := NewActionRegistry(25*time.Millisecond, 1)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	if err := r.Register("stuck", func(context.Context, json.RawMessage) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan protocol.Event, 1)
	go func() { firstDone <- r.Dispatch(context.Background(), "n", "stuck", nil) }()
	<-started
	first := <-firstDone
	if first.Code != "action.timeout" {
		t.Fatalf("first=%+v", first)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	second := r.Dispatch(ctx, "n", "stuck", nil)
	if second.Code != "action.cancelled" {
		t.Fatalf("second request entered saturated executor: %+v", second)
	}
	close(release)
}

func TestParentCancellationDuringHandlerIsNotReportedAsTimeout(t *testing.T) {
	r := NewActionRegistry(time.Second, 1)
	if err := r.Register("wait", func(ctx context.Context, _ json.RawMessage) error {
		<-ctx.Done()
		return ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	ev := r.Dispatch(ctx, "n", "wait", nil)
	if ev.Code != "action.cancelled" {
		t.Fatalf("got %+v", ev)
	}
}

func TestCriticalBatchEnqueueIsAtomic(t *testing.T) {
	b := NewEventBroker(2)
	if err := b.Enqueue(protocol.Event{Ev: "submit", ID: "existing"}, EventCritical); err != nil {
		t.Fatal(err)
	}
	err := b.EnqueueCriticalBatch([]protocol.Event{{Ev: "committed"}, {Ev: "committed"}})
	if err == nil || err.Code != "runtime.backpressure_exceeded" {
		t.Fatalf("got %#v", err)
	}
	if b.Depth() != 1 {
		t.Fatalf("failed batch partially enqueued: depth=%d", b.Depth())
	}
}

func TestActionTimeoutAlsoBoundsWaitingForInflightSlot(t *testing.T) {
	r := NewActionRegistry(25*time.Millisecond, 1)
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	if err := r.Register("stuck-slot", func(context.Context, json.RawMessage) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan protocol.Event, 1)
	go func() { firstDone <- r.Dispatch(context.Background(), "n1", "stuck-slot", nil) }()
	<-started
	first := <-firstDone
	if first.Code != "action.timeout" {
		close(release)
		t.Fatalf("first=%+v", first)
	}

	secondDone := make(chan protocol.Event, 1)
	go func() { secondDone <- r.Dispatch(context.Background(), "n2", "stuck-slot", nil) }()
	select {
	case second := <-secondDone:
		close(release)
		if second.Code != "action.timeout" {
			t.Fatalf("waiting for saturated slot must consume action timeout: %+v", second)
		}
	case <-time.After(100 * time.Millisecond):
		close(release)
		<-secondDone
		t.Fatal("dispatch waited indefinitely for an inflight slot")
	}
}

func TestActionHandlerPanicBecomesFailedEvent(t *testing.T) {
	r := NewActionRegistry(time.Second, 1)
	if err := r.Register("panic", func(context.Context, json.RawMessage) error {
		panic("boom")
	}); err != nil {
		t.Fatal(err)
	}
	ev := r.Dispatch(context.Background(), "node", "panic", nil)
	if ev.Ev != "error" || ev.Code != "action.failed" {
		t.Fatalf("panic escaped action boundary: %+v", ev)
	}
}
