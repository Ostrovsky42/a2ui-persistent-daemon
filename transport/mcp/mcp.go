package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"

	"a2ui/protocol"
	"a2ui/wire"
)

type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// DecodeMessage strictly decodes an MCP JSON-RPC message before routing it.
// Callers receiving raw MCP JSON should prefer this helper over json.Unmarshal.
func DecodeMessage(raw []byte) (Message, *protocol.Error) {
	var msg Message
	if perr := wire.StrictUnmarshal(raw, &msg); perr != nil {
		return Message{}, perr
	}
	return msg, nil
}

func methodFor(k protocol.EnvelopeKind) (string, bool) {
	switch k {
	case protocol.KindHello:
		return "a2ui/hello", true
	case protocol.KindHelloAck:
		return "a2ui/hello_ack", true
	case protocol.KindOperation:
		return "a2ui/operation", true
	case protocol.KindEvent:
		return "a2ui/event", true
	case protocol.KindTelemetry:
		return "a2ui/telemetry", true
	}
	return "", false
}
func kindFor(m string) (protocol.EnvelopeKind, bool) {
	switch m {
	case "a2ui/hello":
		return protocol.KindHello, true
	case "a2ui/hello_ack":
		return protocol.KindHelloAck, true
	case "a2ui/operation":
		return protocol.KindOperation, true
	case "a2ui/event":
		return protocol.KindEvent, true
	case "a2ui/telemetry":
		return protocol.KindTelemetry, true
	}
	return "", false
}

func Notification(env protocol.Envelope) (Message, error) {
	m, ok := methodFor(env.Kind)
	if !ok {
		return Message{}, fmt.Errorf("unsupported envelope kind %q", env.Kind)
	}
	b, err := wire.EncodeEnvelope(env)
	if err != nil {
		return Message{}, err
	}
	return Message{JSONRPC: "2.0", Method: m, Params: b}, nil
}
func Request(id json.RawMessage, env protocol.Envelope) (Message, error) {
	msg, err := Notification(env)
	if err != nil {
		return Message{}, err
	}
	msg.ID = append(json.RawMessage(nil), id...)
	return msg, nil
}
func EnvelopeFromMessage(msg Message) (protocol.Envelope, *protocol.Error) {
	if msg.JSONRPC != "2.0" {
		return protocol.Envelope{}, protocol.NewError("mcp.invalid_jsonrpc", "jsonrpc must be 2.0")
	}
	expected, ok := kindFor(msg.Method)
	if !ok {
		return protocol.Envelope{}, protocol.NewError("mcp.unsupported_method", fmt.Sprintf("unsupported method %q", msg.Method))
	}
	var envelopeShape protocol.Envelope
	if perr := wire.StrictUnmarshal(msg.Params, &envelopeShape); perr != nil {
		return protocol.Envelope{}, perr
	}
	env, perr := wire.DecodeRecord(msg.Params)
	if perr != nil {
		return protocol.Envelope{}, perr
	}
	if env.Kind != expected {
		return protocol.Envelope{}, protocol.NewError("mcp.kind_mismatch", "method and envelope kind disagree")
	}
	return env, nil
}

const MCPProtocolVersion = "2026-07-28"

// HTTPHeaders returns the modern stateless MCP Streamable HTTP routing headers
// for an A2UI bridge message. A2UI state is carried explicitly in Envelope.Session;
// it is not hidden in an MCP transport session.
func HTTPHeaders(msg Message) http.Header {
	h := make(http.Header)
	h.Set("MCP-Protocol-Version", MCPProtocolVersion)
	h.Set("Mcp-Method", msg.Method)
	return h
}

// ValidateHTTPHeaders rejects disagreement between routable MCP headers and the
// JSON-RPC body. This mirrors the 2026-07-28 transport hardening rule.
func ValidateHTTPHeaders(h http.Header, msg Message) *protocol.Error {
	if h.Get("MCP-Protocol-Version") != MCPProtocolVersion {
		return protocol.NewError("mcp.version_mismatch", "MCP-Protocol-Version header mismatch")
	}
	if h.Get("Mcp-Method") != msg.Method {
		return protocol.NewError("mcp.header_mismatch", "Mcp-Method header disagrees with JSON-RPC method")
	}
	return nil
}
