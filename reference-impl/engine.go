// Package a2ui provides the small, buildable reference facade shipped with the
// protocol bundle. The authoritative implementation lives in the renderer-
// neutral core packages; terminal renderers are adapters, not protocol state.
package a2ui

import (
	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

// NewVerifiedEngine returns the stdlib-only reference runtime with finite
// protocol limits, a bounded reliable event queue and an empty action registry.
func NewVerifiedEngine() *engine.Engine {
	limits := protocol.DefaultLimits()
	return engine.New(limits, limits.MaxPendingEvents, engine.NewNoopActions())
}
