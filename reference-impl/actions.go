package a2ui

import (
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/external"
	a2runtime "github.com/Ostrovsky42/agent-interaction-runtime/runtime"
)

// ActionRegistry is re-exported for the compact reference facade.
type ActionRegistry = a2runtime.ActionRegistry

// NewActionRegistry creates a bounded, timeout-aware action registry.
func NewActionRegistry(timeout time.Duration, maxInflight int) *ActionRegistry {
	return a2runtime.NewActionRegistry(timeout, maxInflight)
}

// CDriverWorker is the thread-affine external actor used for CGO/native APIs.
type CDriverWorker = external.Actor

// StartCDriverWorker starts a worker pinned to one OS thread. The executor is
// deliberately injected so the lifecycle can be tested without requiring CGO.
func StartCDriverWorker(queue int, executor external.Executor) *CDriverWorker {
	return external.Start(queue, executor)
}
