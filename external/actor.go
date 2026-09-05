package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

type HealthState uint32

const (
	Ready HealthState = iota + 1
	Degraded
	Closed
)

var ErrClosed = errors.New("external actor closed")
var ErrDegraded = errors.New("external actor degraded after an in-flight timeout")

type Response struct {
	Status int
	Data   json.RawMessage
}
type Executor func(command int, args json.RawMessage) (Response, error)
type result struct {
	response Response
	err      error
}
type request struct {
	command int
	args    json.RawMessage
	result  chan result
}

type Actor struct {
	req      chan request
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	health   atomic.Uint32
}

func Start(queue int, exec Executor) *Actor {
	if queue < 1 {
		queue = 1
	}
	if exec == nil {
		exec = func(int, json.RawMessage) (Response, error) { return Response{}, errors.New("nil executor") }
	}
	a := &Actor{req: make(chan request, queue), stop: make(chan struct{}), done: make(chan struct{})}
	a.health.Store(uint32(Ready))
	go a.run(exec)
	return a
}

func (a *Actor) run(exec Executor) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(a.done)
	for {
		select {
		case req := <-a.req:
			switch a.Health() {
			case Closed:
				req.result <- result{err: ErrClosed}
				continue
			case Degraded:
				req.result <- result{err: ErrDegraded}
				continue
			}
			resp, err, panicked := callExecutor(exec, req.command, req.args)
			if panicked {
				a.health.Store(uint32(Degraded))
			}
			req.result <- result{resp, err}
		case <-a.stop:
			for {
				select {
				case req := <-a.req:
					req.result <- result{err: ErrClosed}
				default:
					return
				}
			}
		}
	}
}

func callExecutor(exec Executor, command int, args json.RawMessage) (response Response, err error, panicked bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = Response{}
			err = fmt.Errorf("external executor panic: %v", recovered)
			panicked = true
		}
	}()
	response, err = exec(command, args)
	return response, err, false
}

func (a *Actor) Health() HealthState {
	if a == nil {
		return Closed
	}
	return HealthState(a.health.Load())
}

func (a *Actor) Call(ctx context.Context, command int, args json.RawMessage) (Response, error) {
	if a == nil {
		return Response{}, ErrClosed
	}
	switch a.Health() {
	case Closed:
		return Response{}, ErrClosed
	case Degraded:
		return Response{}, ErrDegraded
	}
	resultCh := make(chan result, 1)
	req := request{command: command, args: append(json.RawMessage(nil), args...), result: resultCh}
	select {
	case a.req <- req:
	case <-a.stop:
		return Response{}, ErrClosed
	case <-ctx.Done():
		return Response{}, ctx.Err()
	}
	select {
	case r := <-resultCh:
		return r.response, r.err
	case <-a.stop:
		return Response{}, ErrClosed
	case <-ctx.Done():
		a.health.CompareAndSwap(uint32(Ready), uint32(Degraded))
		return Response{}, ctx.Err()
	}
}

func (a *Actor) Close(ctx context.Context) error {
	if a == nil {
		return nil
	}
	a.health.Store(uint32(Closed))
	a.stopOnce.Do(func() { close(a.stop) })
	select {
	case <-a.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
