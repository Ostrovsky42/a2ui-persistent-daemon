package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"a2ui/protocol"
	"a2ui/transport/mcp"
)

func TestDaemonHandlesMCPWithoutOwningRenderer(t *testing.T) {
	d := New("mcp-daemon", protocol.DefaultLimits(), nil)
	helloPayload, _ := json.Marshal(protocol.Hello{Versions: []int{protocol.Version}, Features: []string{"commit-barrier"}})
	helloEnv := protocol.Envelope{V: protocol.Version, Session: "mcp-daemon", Kind: protocol.KindHello, Payload: helloPayload}
	hello, err := mcp.Request(json.RawMessage(`"h1"`), helloEnv)
	if err != nil {
		t.Fatal(err)
	}
	resp, perr := d.HandleMCPMessage(hello)
	if perr != nil {
		t.Fatal(perr)
	}
	if resp == nil || resp.Method != "a2ui/hello_ack" {
		t.Fatalf("hello response=%+v", resp)
	}

	op := protocol.Operation{V: protocol.Version, Seq: 1, Op: protocol.OpUpsert, ID: "title", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"daemon"}`)}
	opPayload, _ := json.Marshal(op)
	opEnv := protocol.Envelope{V: protocol.Version, Session: "mcp-daemon", Kind: protocol.KindOperation, Seq: 1, Payload: opPayload}
	note, err := mcp.Notification(opEnv)
	if err != nil {
		t.Fatal(err)
	}
	if resp, perr := d.HandleMCPMessage(note); perr != nil || resp != nil {
		t.Fatalf("op resp=%+v err=%v", resp, perr)
	}
	if _, ok := d.Engine.Document().Nodes["title"]; !ok {
		t.Fatal("MCP operation did not reach daemon Engine")
	}
}

func TestDaemonServeHTTPUsesExistingMCPContract(t *testing.T) {
	d := New("mcp-http", protocol.DefaultLimits(), nil)
	payload, _ := json.Marshal(protocol.Hello{Versions: []int{protocol.Version}, Features: []string{"commit-barrier"}})
	env := protocol.Envelope{V: protocol.Version, Session: "mcp-http", Kind: protocol.KindHello, Payload: payload}
	msg, err := mcp.Request(json.RawMessage(`"h-http"`), env)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(msg)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	for k, vals := range mcp.HTTPHeaders(msg) {
		for _, v := range vals {
			req.Header.Add(k, v)
		}
	}
	w := httptest.NewRecorder()
	d.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	decoded, perr := mcp.DecodeMessage(w.Body.Bytes())
	if perr != nil {
		t.Fatal(perr)
	}
	if decoded.Method != "a2ui/hello_ack" {
		t.Fatalf("method=%q", decoded.Method)
	}
}

func TestDaemonServeHTTPStatusAndEvents(t *testing.T) {
	d := New("test-session", protocol.DefaultLimits(), nil)

	// Test GET /status
	reqStatus := httptest.NewRequest(http.MethodGet, "/status", nil)
	wStatus := httptest.NewRecorder()
	d.ServeHTTP(wStatus, reqStatus)
	if wStatus.Code != http.StatusOK {
		t.Fatalf("status code=%d", wStatus.Code)
	}
	var status map[string]any
	if err := json.Unmarshal(wStatus.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if status["session"] != "test-session" {
		t.Fatalf("session=%v, want test-session", status["session"])
	}

	// Apply node and simulate submit interaction to generate an event
	_ = d.Engine.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "in1",
		Type:   protocol.NodeInput,
		Parent: "root",
	})
	_ = d.Engine.Submit("in1")

	// Test GET /events
	reqEvents := httptest.NewRequest(http.MethodGet, "/events?timeout=100ms", nil)
	wEvents := httptest.NewRecorder()
	d.ServeHTTP(wEvents, reqEvents)
	if wEvents.Code != http.StatusOK {
		t.Fatalf("events code=%d", wEvents.Code)
	}
	var events []protocol.Event
	if err := json.Unmarshal(wEvents.Body.Bytes(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Ev != "submit" || events[0].ID != "in1" {
		t.Fatalf("unexpected events: %+v", events)
	}

	// Enqueue another event and test MCP wait_event
	_ = d.Engine.Submit("in1")
	waitMsg := mcp.Message{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"w1"`),
		Method:  "a2ui/wait_event",
	}
	resp, perr := d.HandleMCPMessage(waitMsg)
	if perr != nil {
		t.Fatal(perr)
	}
	if resp == nil || resp.Error != nil {
		t.Fatalf("wait_event failed: %+v", resp)
	}
	var res struct {
		Events []protocol.Event `json:"events"`
	}
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Events) != 1 || res.Events[0].Ev != "submit" {
		t.Fatalf("unexpected wait_event result: %+v", res)
	}
}
