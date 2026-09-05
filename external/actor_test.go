package external

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestActorSerializesExecution(t *testing.T) {
	var current, max atomic.Int32
	a := Start(8, func(cmd int, args json.RawMessage) (Response, error) {
		n := current.Add(1)
		defer current.Add(-1)
		for {
			old := max.Load()
			if n <= old || max.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		return Response{Status: cmd}, nil
	})
	defer a.Close(context.Background())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r, err := a.Call(context.Background(), i, nil)
			if err != nil || r.Status != i {
				t.Errorf("r=%+v err=%v", r, err)
			}
		}(i)
	}
	wg.Wait()
	if max.Load() != 1 {
		t.Fatalf("max concurrent=%d", max.Load())
	}
}

func TestTimeoutAfterSubmissionMarksActorDegraded(t *testing.T) {
	release := make(chan struct{})
	a := Start(1, func(int, json.RawMessage) (Response, error) { <-release; return Response{}, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := a.Call(ctx, 1, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if a.Health() != Degraded {
		t.Fatalf("health=%v", a.Health())
	}
	close(release)
	time.Sleep(10 * time.Millisecond)
	_ = a.Close(context.Background())
}

func TestBoundedQueueHonorsCallerContext(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 1)
	a := Start(1, func(int, json.RawMessage) (Response, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-release
		return Response{}, nil
	})
	go a.Call(context.Background(), 1, nil)
	<-started
	go a.Call(context.Background(), 2, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	_, err := a.Call(ctx, 3, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	_ = a.Close(context.Background())
}

func TestCloseRejectsFutureCalls(t *testing.T) {
	a := Start(1, func(int, json.RawMessage) (Response, error) { return Response{}, nil })
	if err := a.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := a.Call(context.Background(), 1, nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("err=%v", err)
	}
}

func TestExecutorPanicReturnsErrorAndDegradesActor(t *testing.T) {
	a := Start(1, func(int, json.RawMessage) (Response, error) {
		panic("native wrapper boom")
	})
	defer a.Close(context.Background())
	_, err := a.Call(context.Background(), 1, nil)
	if err == nil {
		t.Fatal("executor panic returned nil error")
	}
	if a.Health() != Degraded {
		t.Fatalf("health=%v", a.Health())
	}
	_, err = a.Call(context.Background(), 2, nil)
	if !errors.Is(err, ErrDegraded) {
		t.Fatalf("degraded actor accepted another call: %v", err)
	}
}
