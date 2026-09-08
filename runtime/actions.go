package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

type ActionHandler func(context.Context, json.RawMessage) error

type ActionRegistry struct {
	mu       sync.RWMutex
	handlers map[string]ActionHandler
	timeout  time.Duration
	sem      chan struct{}
	counter  atomic.Uint64
}

func NewActionRegistry(timeout time.Duration, maxInflight int) *ActionRegistry {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if maxInflight < 1 {
		maxInflight = 1
	}
	return &ActionRegistry{handlers: map[string]ActionHandler{}, timeout: timeout, sem: make(chan struct{}, maxInflight)}
}

func (r *ActionRegistry) Register(id string, h ActionHandler) error {
	if r == nil {
		return fmt.Errorf("nil registry")
	}
	if id == "" {
		return fmt.Errorf("empty action id")
	}
	if h == nil {
		return fmt.Errorf("nil handler for %s", id)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[id]; exists {
		return fmt.Errorf("action %s already registered", id)
	}
	r.handlers[id] = h
	return nil
}

func (r *ActionRegistry) Keys() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.handlers))
	for k := range r.handlers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (r *ActionRegistry) Dispatch(parent context.Context, nodeID, actionID string, args json.RawMessage) protocol.Event {
	if r == nil {
		return protocol.Event{V: protocol.Version, Ev: "error", ID: nodeID, Action: actionID, Code: "action.not_permitted", Msg: "action registry unavailable"}
	}
	if parent == nil {
		parent = context.Background()
	}
	invocation := fmt.Sprintf("a-%d", r.counter.Add(1))
	base := protocol.Event{V: protocol.Version, ID: nodeID, Action: actionID, InvocationID: invocation, Args: append(json.RawMessage(nil), args...)}
	r.mu.RLock()
	h, ok := r.handlers[actionID]
	r.mu.RUnlock()
	if !ok {
		base.Ev = "error"
		base.Code = "action.not_permitted"
		base.Msg = "action is not registered"
		return base
	}
	if err := parent.Err(); err != nil {
		base.Ev = "error"
		base.Code = "action.cancelled"
		base.Msg = err.Error()
		return base
	}
	ctx, cancel := context.WithTimeout(parent, r.timeout)
	defer cancel()
	classifyContext := func(err error) protocol.Event {
		base.Ev = "error"
		if parent.Err() != nil {
			base.Code = "action.cancelled"
			base.Msg = parent.Err().Error()
		} else {
			base.Code = "action.timeout"
			base.Msg = err.Error()
		}
		return base
	}
	select {
	case r.sem <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-r.sem
			return classifyContext(err)
		}
	case <-ctx.Done():
		return classifyContext(ctx.Err())
	}
	argsCopy := append(json.RawMessage(nil), args...)
	done := make(chan error, 1)
	go func() {
		defer func() { <-r.sem }()
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- fmt.Errorf("action handler panic: %v", recovered)
			}
		}()
		done <- h(ctx, argsCopy)
	}()
	select {
	case err := <-done:
		if err != nil {
			if ctx.Err() != nil {
				return classifyContext(ctx.Err())
			}
			base.Ev = "error"
			base.Code = "action.failed"
			base.Msg = err.Error()
			return base
		}
		base.Ev = "action_result"
		return base
	case <-ctx.Done():
		return classifyContext(ctx.Err())
	}
}
