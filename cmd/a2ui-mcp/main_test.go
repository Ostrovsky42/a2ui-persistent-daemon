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
	var operations []protocol.Operation

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				operations = append(operations, op)
			}
			mu.Unlock()
			result, _ := json.Marshal(map[string]any{"published": len(batch.Operations)})
			_ = json.NewEncoder(w).Encode(transportmcp.Message{JSONRPC: "2.0", ID: msg.ID, Result: result})
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
			http.Error(w, "unexpected envelope", http.StatusBadRequest)
		}
	}))
	defer daemon.Close()

	a2client := agentclient.New(daemon.URL, "p0", daemon.Client())
	session := connectMCP(t, newMCPServer(a2client))
	ctx := context.Background()

	first, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "a2ui_publish",
		Arguments: map[string]any{"operations": []any{
			map[string]any{
				"op":     "upsert",
				"id":     "targets",
				"type":   "table",
				"parent": "root",
				"props": map[string]any{
					"columns": []any{
						map[string]any{"title": "Target", "width": 18},
						map[string]any{"title": "State", "width": 10},
					},
					"rows": []any{
						[]any{"staging", "ready"},
						[]any{"production", "guarded"},
					},
					"row_ids":    []any{"staging", "production"},
					"selectable": true,
					"action":     "deployment.select",
				},
			},
			map[string]any{"op": "focus", "id": "targets"},
			map[string]any{"op": "commit", "frame": "deployment-choice"},
		}},
	})
	if err != nil {
		t.Fatalf("first a2ui_publish: %v", err)
	}
	if first.IsError {
		t.Fatalf("first a2ui_publish returned tool error: %#v", first.Content)
	}

	second, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "a2ui_publish",
		Arguments: map[string]any{"operations": []any{
			map[string]any{"op": "text", "id": "status", "text": "staging selected"},
			map[string]any{"op": "commit", "frame": "deployment-selected"},
		}},
	})
	if err != nil {
		t.Fatalf("second a2ui_publish: %v", err)
	}
	if second.IsError {
		t.Fatalf("second a2ui_publish returned tool error: %#v", second.Content)
	}

	mu.Lock()
	defer mu.Unlock()
	if helloCount != 1 {
		t.Fatalf("hello count = %d, want 1", helloCount)
	}
	want := []uint64{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(sequences, want) {
		t.Fatalf("operation sequences = %v, want %v", sequences, want)
	}
	if len(operations) != 5 {
		t.Fatalf("operations = %d, want 5", len(operations))
	}
	var props map[string]any
	if err := json.Unmarshal(operations[0].Props, &props); err != nil {
		t.Fatalf("decode table props: %v", err)
	}
	if props["action"] != "deployment.select" || props["selectable"] != true {
		t.Fatalf("table props = %#v, want selectable deployment action", props)
	}
	if rowIDs, ok := props["row_ids"].([]any); !ok || len(rowIDs) != 2 || rowIDs[0] != "staging" || rowIDs[1] != "production" {
		t.Fatalf("row_ids = %#v, want stable staging/production IDs", props["row_ids"])
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
		_ = json.NewEncoder(w).Encode([]protocol.Event{{V: protocol.Version, Seq: 2, Ev: "select", ID: "targets", Action: "deployment.select", Row: 0, RowID: "staging"}})
	}))
	defer daemon.Close()

	a2client := agentclient.New(daemon.URL, "p0", daemon.Client())
	session := connectMCP(t, newMCPServer(a2client))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "a2ui_wait_event",
		Arguments: map[string]any{
			"timeout_ms":  1000,
			"event_types": []any{"select"},
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
		t.Fatal("wait_event timed out, want select match")
	}
	if len(output.MatchedEvents) != 1 || output.MatchedEvents[0].Ev != "select" || output.MatchedEvents[0].RowID != "staging" {
		t.Fatalf("matched events = %#v, want staging select", output.MatchedEvents)
	}
	if len(output.ObservedEvents) != 2 || output.ObservedEvents[0].Ev != "committed" || output.ObservedEvents[1].Ev != "select" {
		t.Fatalf("observed events = %#v, want committed then select", output.ObservedEvents)
	}
}

func TestStatusToolReturnsExistingDaemonStatus(t *testing.T) {
	t.Parallel()

	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/status" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(agentclient.Status{
			Session:        "p0",
			Revision:       12,
			Nodes:          4,
			HasClient:      true,
			Generation:     9,
			PendingPublish: false,
		})
	}))
	defer daemon.Close()

	a2client := agentclient.New(daemon.URL, "p0", daemon.Client())
	session := connectMCP(t, newMCPServer(a2client))
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "a2ui_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("a2ui_status: %v", err)
	}
	if result.IsError {
		t.Fatalf("a2ui_status returned tool error: %#v", result.Content)
	}

	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var status agentclient.Status
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if status.Session != "p0" || status.Revision != 12 || status.Nodes != 4 || !status.HasClient || status.Generation != 9 || status.PendingPublish {
		t.Fatalf("status = %#v, want decoded daemon status", status)
	}
}
