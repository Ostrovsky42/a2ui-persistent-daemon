package daemon

import (
	"fmt"
	"sync"

	"github.com/Ostrovsky42/agent-interaction-runtime/ipc"
)

type clientLease struct {
	mu     sync.Mutex
	active string
	nextID uint64
}

func (l *clientLease) acquire() (string, *ipc.Error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.active != "" {
		return "", ipc.NewError("ipc.client_busy", "an interactive client is already attached")
	}
	l.nextID++
	l.active = fmt.Sprintf("client-%d", l.nextID)
	return l.active, nil
}

func (l *clientLease) release(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if id == "" || l.active != id {
		return false
	}
	l.active = ""
	return true
}

func (l *clientLease) owner() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.active
}
