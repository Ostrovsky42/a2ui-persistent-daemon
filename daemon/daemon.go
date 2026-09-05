package daemon

import (
	"sync"

	"a2ui/engine"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
	"a2ui/session"
)

// Daemon owns the long-lived semantic A2UI state. Terminal clients are views
// over this state and may disconnect without closing Session or Engine.
type Daemon struct {
	Engine  *engine.Engine
	Session *session.Session
	Actions *a2runtime.ActionRegistry
	limits  protocol.Limits

	agentMu sync.Mutex
	lease   clientLease
	updates chan struct{}

	clientMu   sync.Mutex
	activeConn interface{ Close() error }
}

func New(sessionID string, limits protocol.Limits, actions *a2runtime.ActionRegistry) *Daemon {
	limits = protocol.EffectiveLimits(limits)
	if actions == nil {
		actions = engine.NewNoopActions()
	}
	return &Daemon{
		Engine:  engine.New(limits, limits.MaxPendingEvents, actions),
		Session: session.New(sessionID, limits),
		Actions: actions,
		limits:  limits,
		updates: make(chan struct{}, 1),
	}
}

func (d *Daemon) signalSnapshot() {
	select {
	case d.updates <- struct{}{}:
	default:
	}
}

func (d *Daemon) setActiveConn(conn interface{ Close() error }) {
	d.clientMu.Lock()
	d.activeConn = conn
	d.clientMu.Unlock()
}

func (d *Daemon) clearActiveConn(conn interface{ Close() error }) {
	d.clientMu.Lock()
	if d.activeConn == conn {
		d.activeConn = nil
	}
	d.clientMu.Unlock()
}

func (d *Daemon) closeActiveConn() {
	d.clientMu.Lock()
	conn := d.activeConn
	d.clientMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}
