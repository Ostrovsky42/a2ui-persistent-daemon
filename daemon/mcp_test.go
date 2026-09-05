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
