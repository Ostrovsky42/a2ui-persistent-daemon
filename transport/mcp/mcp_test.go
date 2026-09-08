package mcp

import (
	"encoding/json"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	"testing"
)

func TestEnvelopeMapsToJSONRPCNotificationAndBack(t *testing.T) {
	env := protocol.Envelope{V: 1, Session: "s", Kind: protocol.KindEvent, Seq: 9, Payload: json.RawMessage(`{"v":1,"seq":9,"ev":"resize","w":80,"h":24}`)}
	msg, err := Notification(env)
	if err != nil {
		t.Fatal(err)
	}
	if msg.JSONRPC != "2.0" || msg.Method != "github.com/Ostrovsky42/agent-interaction-runtime/event" || len(msg.ID) != 0 {
		t.Fatalf("msg=%+v", msg)
	}
	got, perr := EnvelopeFromMessage(msg)
	if perr != nil {
		t.Fatal(perr)
	}
	if got.Kind != env.Kind || got.Seq != 9 || got.Session != "s" {
		t.Fatalf("got=%+v", got)
	}
}

func TestUnknownMCPMethodRejected(t *testing.T) {
	_, err := EnvelopeFromMessage(Message{JSONRPC: "2.0", Method: "tools/call", Params: json.RawMessage(`{}`)})
	if err == nil || err.Code != "mcp.unsupported_method" {
		t.Fatalf("got %#v", err)
	}
}

func TestModernMCPHTTPHeadersAreSelfDescribing(t *testing.T) {
	env := protocol.Envelope{V: 1, Session: "a2ui-handle", Kind: protocol.KindOperation, Seq: 1, Payload: json.RawMessage(`{"v":1,"seq":1,"op":"commit"}`)}
	msg, err := Request(json.RawMessage(`1`), env)
	if err != nil {
		t.Fatal(err)
	}
	h := HTTPHeaders(msg)
	if h.Get("MCP-Protocol-Version") != "2026-07-28" || h.Get("Mcp-Method") != "github.com/Ostrovsky42/agent-interaction-runtime/operation" {
		t.Fatalf("headers=%v", h)
	}
	if perr := ValidateHTTPHeaders(h, msg); perr != nil {
		t.Fatal(perr)
	}
	h.Set("Mcp-Method", "tools/call")
	if perr := ValidateHTTPHeaders(h, msg); perr == nil || perr.Code != "mcp.header_mismatch" {
		t.Fatalf("got %#v", perr)
	}
}

func TestEnvelopeFromMessageStrictlyValidatesEnvelopeParams(t *testing.T) {
	cases := []struct {
		params string
		code   string
	}{
		{`{"v":1,"session":"s","kind":"event","seq":1,"payload":{},"wat":1}`, "wire.unknown_field"},
		{`{"v":1,"session":"s","kind":"event","kind":"telemetry","seq":1,"payload":{}}`, "wire.duplicate_key"},
	}
	for _, tc := range cases {
		_, perr := EnvelopeFromMessage(Message{JSONRPC: "2.0", Method: "github.com/Ostrovsky42/agent-interaction-runtime/event", Params: json.RawMessage(tc.params)})
		if perr == nil || perr.Code != tc.code {
			t.Fatalf("params=%s got %#v", tc.params, perr)
		}
	}
}

func TestDecodeMessageRejectsAmbiguousOrUnknownJSONRPCFields(t *testing.T) {
	cases := []struct {
		raw  string
		code string
	}{
		{`{"jsonrpc":"2.0","method":"github.com/Ostrovsky42/agent-interaction-runtime/event","method":"github.com/Ostrovsky42/agent-interaction-runtime/telemetry","params":{}}`, "wire.duplicate_key"},
		{`{"jsonrpc":"2.0","method":"github.com/Ostrovsky42/agent-interaction-runtime/event","params":{},"wat":1}`, "wire.unknown_field"},
	}
	for _, tc := range cases {
		_, perr := DecodeMessage([]byte(tc.raw))
		if perr == nil || perr.Code != tc.code {
			t.Fatalf("raw=%s got %#v", tc.raw, perr)
		}
	}
}

func TestNotificationRejectsInvalidA2UIEnvelope(t *testing.T) {
	env := protocol.Envelope{V: 1, Session: "s", Kind: protocol.KindOperation, Seq: 2, Payload: json.RawMessage(`{"v":1,"seq":1,"op":"commit"}`)}
	if _, err := Notification(env); err == nil {
		t.Fatal("MCP bridge serialized invalid A2UI envelope")
	}
}

func TestEnvelopeFromMessageValidatesNestedA2UIPayload(t *testing.T) {
	msg := Message{
		JSONRPC: "2.0",
		Method:  "github.com/Ostrovsky42/agent-interaction-runtime/operation",
		Params:  json.RawMessage(`{"v":1,"session":"s","kind":"operation","seq":2,"payload":{"v":1,"seq":1,"op":"commit"}}`),
	}
	_, perr := EnvelopeFromMessage(msg)
	if perr == nil || perr.Code != "protocol.sequence_mismatch" {
		t.Fatalf("got %#v", perr)
	}
}
