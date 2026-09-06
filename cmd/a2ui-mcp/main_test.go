package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	"a2ui/agentclient"
	"a2ui/protocol"
	transportmcp "a2ui/transport/mcp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectMCP(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "a2ui-p0-test", Version: "v0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func TestMCPServerExposesExactlyP0Tools(t *testing.T) {
	t.Parallel()

	clientSession := connectMCP(t, newMCPServer(nil))
	result, err := clientSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("tools/list: %v", err)
	}

	got := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		got = append(got, tool.Name)
	}
	sort.Strings(got)
	want := []string{"a2ui_publish", "a2ui_status", "a2ui_wait_event"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tool names = %v, want %v", got, want)
	}
}

func TestPublishToolUsesPersistentA2UIAuthority(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	helloCount := 0
	var sequences []uint64

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var msg transportmcp.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		case protocol.KindOperation:
			sequences = append(sequences, env.Seq)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected envelope", http.StatusBadRequest)
		}
	}))
	defer daemon.Close()

	a2client := agentclient.New(daemon.URL, "p0", daemon.Client())
	session := connectMCP(t, newMCPServer(a2client))
	ctx := context.Background()

	for _, text := range []string{"first", "second"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "a2ui_publish",
			Arguments: map[string]any{"operations": []any{
				map[string]any{"op": "text", "id": "status", "text": text},
				map[string]any{"op": "commit", "frame": text},
			}},
		})
		if err != nil {
			t.Fatalf("a2ui_publish %q: %v", text, err)
		}
		if result.IsError {
			t.Fatalf("a2ui_publish %q returned tool error: %#v", text, result.Content)
		}
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

func TestWaitEventToolFiltersWithoutLosingObservedEvents(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	calls := 0
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		mu.Lock()
		calls++
		call := calls
		mu.Unlock()
		if call == 1 {
			_ = json.NewEncoder(w).Encode([]protocol.Event{{V: protocol.Version, Seq: 1, Ev: "committed", Frame: "choice"}})
			return
		}
		_ = json.NewEncoder(w).Encode([]protocol.Event{{V: protocol.Version, Seq: 2, Ev: "submit", ID: "target", Value: "staging"}})
	}))
	defer daemon.Close()

	a2client := agentclient.New(daemon.URL, "p0", daemon.Client())
	session := connectMCP(t, newMCPServer(a2client))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "a2ui_wait_event",
		Arguments: map[string]any{
			"timeout_ms":  1000,
			"event_types": []any{"submit"},
		},
	})
	if err != nil {
		t.Fatalf("a2ui_wait_event: %v", err)
	}
	if result.IsError {
		t.Fatalf("a2ui_wait_event returned tool error: %#v", result.Content)
	}

	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output struct {
		MatchedEvents  []protocol.Event `json:"matched_events"`
		ObservedEvents []protocol.Event `json:"observed_events"`
		TimedOut       bool             `json:"timed_out"`
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if output.TimedOut {
		t.Fatal("wait_event timed out, want submit match")
	}
	if len(output.MatchedEvents) != 1 || output.MatchedEvents[0].Ev != "submit" || output.MatchedEvents[0].Value != "staging" {
		t.Fatalf("matched events = %#v, want staging submit", output.MatchedEvents)
	}
	if len(output.ObservedEvents) != 2 || output.ObservedEvents[0].Ev != "committed" || output.ObservedEvents[1].Ev != "submit" {
		t.Fatalf("observed events = %#v, want committed then submit", output.ObservedEvents)
	}
}
