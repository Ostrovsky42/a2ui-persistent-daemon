package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"a2ui/daemon"
	"a2ui/protocol"
	transportmcp "a2ui/transport/mcp"
)

func TestPublishNegotiatesOnceAndContinuesSequenceAcrossCalls(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	helloCount := 0
	var sequences []uint64

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		var msg transportmcp.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if msg.Method == "a2ui/publish_batch" {
			var batch struct {
				Operations []protocol.Operation `json:"operations"`
			}
			if err := json.Unmarshal(msg.Params, &batch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			for _, op := range batch.Operations {
				sequences = append(sequences, uint64(op.Seq))
			}
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(transportmcp.Message{JSONRPC: "2.0", ID: msg.ID, Result: json.RawMessage(`{"published":2}`)})
			return
		}
		env, perr := transportmcp.EnvelopeFromMessage(msg)
		if perr != nil {
			http.Error(w, perr.Message, http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch env.Kind {
		case protocol.KindHello:
			helloCount++
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected envelope kind", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := New(server.URL, "p0", server.Client())
	ctx := context.Background()
	if err := client.Publish(ctx, []protocol.Operation{
		{Op: protocol.OpText, ID: "status", Text: "first"},
		{Op: protocol.OpCommit, Frame: "first"},
	}); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if err := client.Publish(ctx, []protocol.Operation{
		{Op: protocol.OpText, ID: "status", Text: "second"},
		{Op: protocol.OpCommit, Frame: "second"},
	}); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if helloCount != 1 {
		t.Fatalf("hello count = %d, want 1", helloCount)
	}
	want := []uint64{1, 2, 3, 4}
	if !reflect.DeepEqual(sequences, want) {
		t.Fatalf("operation sequences = %v, want %v", sequences, want)
	}
}

func TestPublishReturnsDaemonHTTPRejection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "semantic rejection", http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	client := New(server.URL, "p0", server.Client())
	err := client.Publish(context.Background(), []protocol.Operation{{Op: protocol.OpCommit, Frame: "rejected"}})
	if err == nil {
		t.Fatal("Publish succeeded, want daemon rejection")
	}
	if !strings.Contains(err.Error(), "422") || !strings.Contains(err.Error(), "semantic rejection") {
		t.Fatalf("Publish error = %q, want HTTP status and body", err)
	}
}

func TestPublishClassifiesAlreadyNegotiatedSessionAsAgentStreamConflict(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"code":-32000,"message":"hello only allowed in NEW"}`))
	}))
	defer server.Close()

	client := New(server.URL, "reused", server.Client())
	err := client.Publish(context.Background(), []protocol.Operation{{Op: protocol.OpCommit, Frame: "first"}})
	if err == nil {
		t.Fatal("Publish succeeded, want stale agent-stream rejection")
	}
	if !errors.Is(err, ErrAgentStreamConflict) {
		t.Fatalf("Publish error = %v, want ErrAgentStreamConflict", err)
	}
	if !strings.Contains(err.Error(), "reused") || !strings.Contains(err.Error(), "fresh daemon/session") {
		t.Fatalf("Publish error = %q, want actionable session and recovery guidance", err)
	}
}

func TestPublishClassifiesRealDaemonReusedSessionAsAgentStreamConflict(t *testing.T) {
	t.Parallel()

	d := daemon.New("real-reused", protocol.DefaultLimits(), nil)
	server := httptest.NewServer(d)
	defer server.Close()

	ctx := context.Background()
	first := New(server.URL, "real-reused", server.Client())
	if err := first.Publish(ctx, nil); err != nil {
		t.Fatalf("first agent stream negotiate: %v", err)
	}

	second := New(server.URL, "real-reused", server.Client())
	err := second.Publish(ctx, nil)
	if err == nil {
		t.Fatal("second agent stream negotiation succeeded, want explicit conflict")
	}
	if !errors.Is(err, ErrAgentStreamConflict) {
		t.Fatalf("second agent stream error = %v, want ErrAgentStreamConflict", err)
	}
	if !strings.Contains(err.Error(), "real-reused") || !strings.Contains(err.Error(), "fresh daemon/session") {
		t.Fatalf("second agent stream error = %q, want actionable real-session recovery guidance", err)
	}
}

func TestWaitEventsAndStatusUseExistingDaemonEndpoints(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/events":
			if r.URL.Query().Get("timeout") == "" {
				http.Error(w, "missing timeout", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode([]protocol.Event{{V: protocol.Version, Seq: 7, Ev: "submit", ID: "answer", Value: "staging"}})
		case "/status":
			_ = json.NewEncoder(w).Encode(Status{
				Session:        "p0",
				Revision:       12,
				Nodes:          4,
				HasClient:      true,
				Generation:     9,
				PendingPublish: false,
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := New(server.URL, "p0", server.Client())
	events, err := client.WaitEvents(context.Background(), 25*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitEvents: %v", err)
	}
	if len(events) != 1 || events[0].Ev != "submit" || events[0].Value != "staging" {
		t.Fatalf("events = %#v, want one staging submit", events)
	}

	status, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Session != "p0" || status.Revision != 12 || status.Nodes != 4 || !status.HasClient || status.Generation != 9 || status.PendingPublish {
		t.Fatalf("status = %#v, want decoded daemon status", status)
	}
}
