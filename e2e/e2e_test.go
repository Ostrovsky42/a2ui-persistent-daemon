package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"a2ui/layout"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
	"a2ui/session"
	"a2ui/transport/mcp"
)

func TestMCPInteractiveFormScenario(t *testing.T) {
	runner := NewRunner("sess-form-001", protocol.DefaultLimits(), 80, 24, nil)
	agent := NewAgentSimulator("sess-form-001", runner)

	// Step 1: Handshake
	if err := agent.Connect("commit-barrier", "incremental-props", "action-events"); err != nil {
		t.Fatalf("agent connect failed: %v", err)
	}
	if agent.Ack == nil || agent.Ack.Version != protocol.Version {
		t.Fatalf("unexpected hello_ack: %+v", agent.Ack)
	}

	// Step 2: Agent builds the interactive form
	if err := agent.SetupFormScreen(); err != nil {
		t.Fatalf("setup form screen failed: %v", err)
	}

	frame1 := runner.LastFrame()
	if frame1 == nil {
		t.Fatal("expected frame after commit, got nil")
	}
	text1 := frame1.PlainText()
	if !strings.Contains(text1, "A2UI Agent Service Setup") {
		t.Fatalf("frame1 missing title, got:\n%s", text1)
	}
	if !strings.Contains(text1, "Please provide operator identity:") {
		t.Fatalf("frame1 missing label, got:\n%s", text1)
	}
	if !strings.Contains(text1, "[s: Submit]") {
		t.Fatalf("frame1 missing submit action, got:\n%s", text1)
	}

	// Step 3: User interacts - types username
	if err := runner.TypeInput("username", "AgentSmith"); err != nil {
		t.Fatalf("type input failed: %v", err)
	}

	frame2 := runner.LastFrame()
	if frame2 == nil {
		t.Fatal("expected frame2 after typing input, got nil")
	}
	text2 := frame2.PlainText()
	if !strings.Contains(text2, "AgentSmith") {
		t.Fatalf("frame2 does not reflect typed input, got:\n%s", text2)
	}

	// Step 4: User submits
	if err := runner.SubmitInput("username"); err != nil {
		t.Fatalf("submit input failed: %v", err)
	}

	// Step 5: Agent receives submit event via MCP
	events, err := agent.FetchEvents()
	if err != nil {
		t.Fatalf("fetch events failed: %v", err)
	}
	var submitEv *protocol.Event
	for _, ev := range events {
		if ev.Ev == "submit" {
			submitEv = &ev
			break
		}
	}
	if submitEv == nil {
		t.Fatal("agent did not receive submit event")
	}
	if submitEv.Value != "AgentSmith" || submitEv.Action != "submit_login" {
		t.Fatalf("unexpected submit event payload: %+v", submitEv)
	}

	// Step 6: Agent updates UI with success view
	if err := agent.CompleteFormSuccess(submitEv.Value); err != nil {
		t.Fatalf("complete form success failed: %v", err)
	}

	frame3 := runner.LastFrame()
	if frame3 == nil {
		t.Fatal("expected frame3 after success commit, got nil")
	}
	text3 := frame3.PlainText()
	if !strings.Contains(text3, "Welcome, AgentSmith!") {
		t.Fatalf("frame3 missing welcome message, got:\n%s", text3)
	}
	if !strings.Contains(text3, "100% Ready") {
		t.Fatalf("frame3 missing progress bar, got:\n%s", text3)
	}

	// Step 7: Verify commit barrier event
	commitEvents, err := agent.FetchEvents()
	if err != nil {
		t.Fatalf("fetch commit events failed: %v", err)
	}
	committedFound := false
	for _, ev := range commitEvents {
		if ev.Ev == "committed" && ev.Frame == "form-success" {
			committedFound = true
			if ev.ThroughSeq == 0 {
				t.Fatalf("expected positive ThroughSeq, got %d", ev.ThroughSeq)
			}
		}
	}
	if !committedFound {
		t.Fatal("agent did not receive 'committed' event for frame 'form-success'")
	}
}

func TestMCPStreamingTokensAndProgressBar(t *testing.T) {
	runner := NewRunner("sess-stream-002", protocol.DefaultLimits(), 80, 24, nil)
	agent := NewAgentSimulator("sess-stream-002", runner)

	if err := agent.Connect(); err != nil {
		t.Fatal(err)
	}

	// Container box
	if err := agent.Upsert("stream-box", "root", protocol.NodeBox, map[string]any{
		"dir":     string(layout.Column),
		"border":  string(layout.BorderNormal),
		"padding": 1,
		"gap":     1,
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Progress node
	if err := agent.Upsert("stream-pbar", "stream-box", protocol.NodeProgress, map[string]any{
		"value": 0.0,
		"label": "Streaming tokens...",
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Text node
	if err := agent.Upsert("stream-text", "stream-box", protocol.NodeText, map[string]any{
		"text": "",
		"style": map[string]any{
			"fg": "primary",
		},
	}, nil); err != nil {
		t.Fatal(err)
	}

	if err := agent.Commit("init-stream"); err != nil {
		t.Fatal(err)
	}

	tokens := []string{"Model", " streaming", " inference", " active.", " Output", " validated."}
	for i, tok := range tokens {
		if err := agent.AppendText("stream-text", tok); err != nil {
			t.Fatalf("token %d failed: %v", i, err)
		}
		progressVal := float64(i+1) / float64(len(tokens))
		if err := agent.UpdateProps("stream-pbar", map[string]any{
			"value": progressVal,
			"label": fmt.Sprintf("Token %d/%d", i+1, len(tokens)),
		}); err != nil {
			t.Fatalf("update pbar %d failed: %v", i, err)
		}
		if err := agent.Commit(fmt.Sprintf("frame-tok-%d", i+1)); err != nil {
			t.Fatalf("commit %d failed: %v", i, err)
		}
	}

	finalFrame := runner.LastFrame()
	if finalFrame == nil {
		t.Fatal("expected final frame, got nil")
	}
	text := finalFrame.PlainText()
	if !strings.Contains(text, "Model streaming inference active. Output validated.") {
		t.Fatalf("stream text missing, got:\n%s", text)
	}
	if !strings.Contains(text, "100% Token 6/6") {
		t.Fatalf("progress bar missing 100%%, got:\n%s", text)
	}

	// Verify all commit events arrived
	events, err := agent.FetchEvents()
	if err != nil {
		t.Fatal(err)
	}
	commitCount := 0
	for _, ev := range events {
		if ev.Ev == "committed" {
			commitCount++
		}
	}
	// 1 initial + 6 token commits = 7
	if commitCount != 7 {
		t.Fatalf("expected 7 commit events, got %d", commitCount)
	}
}

func TestMCPHTTPStreamableEndpoint(t *testing.T) {
	runner := NewRunner("sess-http-003", protocol.DefaultLimits(), 80, 24, nil)
	ts := httptest.NewServer(runner)
	defer ts.Close()

	// 1. Send valid a2ui/hello request
	helloEnv := protocol.Envelope{
		V:       1,
		Session: "sess-http-003",
		Kind:    protocol.KindHello,
		Payload: json.RawMessage(`{"versions":[1],"features":["commit-barrier"]}`),
	}
	helloMsg, err := mcp.Request(json.RawMessage(`1`), helloEnv)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(helloMsg)

	req, _ := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(body))
	for k, v := range mcp.HTTPHeaders(helloMsg) {
		req.Header[k] = v
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("expected 200 OK, got %d: %s", res.StatusCode, string(b))
	}
	if res.Header.Get("MCP-Protocol-Version") != "2026-07-28" {
		t.Fatalf("missing or bad MCP-Protocol-Version: %s", res.Header.Get("MCP-Protocol-Version"))
	}
	if res.Header.Get("Mcp-Method") != "a2ui/hello_ack" {
		t.Fatalf("expected Mcp-Method a2ui/hello_ack, got %s", res.Header.Get("Mcp-Method"))
	}

	// 2. Test header mismatch rejection
	reqBad, _ := http.NewRequest(http.MethodPost, ts.URL, bytes.NewReader(body))
	reqBad.Header.Set("MCP-Protocol-Version", "2026-07-28")
	reqBad.Header.Set("Mcp-Method", "tools/call") // Mismatch!

	resBad, err := http.DefaultClient.Do(reqBad)
	if err != nil {
		t.Fatal(err)
	}
	defer resBad.Body.Close()
	if resBad.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for mismatched header, got %d", resBad.StatusCode)
	}
}

func TestMCPErrorHandlingAndBackpressure(t *testing.T) {
	runner := NewRunner("sess-err-004", protocol.DefaultLimits(), 80, 24, nil)

	// Step 1: Negotiate
	helloEnv := protocol.Envelope{
		V:       1,
		Session: "sess-err-004",
		Kind:    protocol.KindHello,
		Payload: json.RawMessage(`{"versions":[1]}`),
	}
	hMsg, err := mcp.Notification(helloEnv)
	if err != nil {
		t.Fatal(err)
	}
	if _, perr := runner.HandleMCPMessage(hMsg); perr != nil {
		t.Fatal(perr)
	}

	// Step 2: Inject sequence gap (seq: 10 instead of 1)
	gapEnv := protocol.Envelope{
		V:       1,
		Session: "sess-err-004",
		Kind:    protocol.KindOperation,
		Seq:     10,
		Payload: json.RawMessage(`{"v":1,"seq":10,"op":"commit"}`),
	}
	gapMsg, err := mcp.Notification(gapEnv)
	if err != nil {
		t.Fatal(err)
	}
	_, perr := runner.HandleMCPMessage(gapMsg)
	if perr == nil || perr.Code != "protocol.sequence_gap" {
		t.Fatalf("expected protocol.sequence_gap error, got %#v", perr)
	}
	if runner.Session.State() != session.Closed {
		t.Fatalf("session should be Closed after sequence gap, got %v", runner.Session.State())
	}
}

func TestVirtualTerminalLayoutMatrix(t *testing.T) {
	runner := NewRunner("sess-term-005", protocol.DefaultLimits(), 80, 24, nil)
	agent := NewAgentSimulator("sess-term-005", runner)

	if err := agent.Connect(); err != nil {
		t.Fatal(err)
	}

	// Outer box with border
	if err := agent.Upsert("main", "root", protocol.NodeBox, map[string]any{
		"dir":     "col",
		"border":  "rounded",
		"padding": 1,
		"gap":     1,
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Table inside
	if err := agent.Upsert("table1", "main", protocol.NodeTable, map[string]any{
		"columns": []map[string]any{
			{"title": "Service", "width": 14},
			{"title": "Status", "width": 10},
			{"title": "Latency", "width": 10},
		},
		"rows": [][]string{
			{"auth-service", "OK", "12ms"},
			{"billing-api", "DEGRADED", "145ms"},
			{"mcp-bridge", "OK", "3ms"},
		},
	}, nil); err != nil {
		t.Fatal(err)
	}

	if err := agent.Commit("table-frame"); err != nil {
		t.Fatal(err)
	}

	f := runner.LastFrame()
	if f == nil {
		t.Fatal("expected frame, got nil")
	}
	text := f.PlainText()
	if !strings.Contains(text, "Service") || !strings.Contains(text, "auth-service") {
		t.Fatalf("table missing columns or data, got:\n%s", text)
	}
	if !strings.Contains(text, "╭") || !strings.Contains(text, "╯") {
		t.Fatalf("rounded borders missing, got:\n%s", text)
	}
}

func TestActionKeyDispatchIntegration(t *testing.T) {
	actions := a2runtime.NewActionRegistry(2*time.Second, 4)
	actionFired := false
	_ = actions.Register("toggle_flag", func(ctx context.Context, args json.RawMessage) error {
		actionFired = true
		return nil
	})

	runner := NewRunner("sess-act-006", protocol.DefaultLimits(), 80, 24, actions)
	agent := NewAgentSimulator("sess-act-006", runner)

	if err := agent.Connect(); err != nil {
		t.Fatal(err)
	}

	if err := agent.Upsert("act-node", "root", protocol.NodeActions, map[string]any{
		"items": []map[string]any{
			{"key": "t", "label": "Toggle", "action": "toggle_flag"},
		},
	}, nil); err != nil {
		t.Fatal(err)
	}

	if err := agent.Commit("actions-ready"); err != nil {
		t.Fatal(err)
	}

	// Press key 't'
	if err := runner.PressKey(context.Background(), "t"); err != nil {
		t.Fatalf("press key failed: %v", err)
	}
	if !actionFired {
		t.Fatal("expected action toggle_flag to fire")
	}

	// Check action_result event queued
	events := runner.DrainEvents()
	resultFound := false
	for _, ev := range events {
		if ev.Ev == "action_result" && ev.Action == "toggle_flag" {
			resultFound = true
		}
	}
	if !resultFound {
		t.Fatalf("action_result event not found in: %+v", events)
	}
}
