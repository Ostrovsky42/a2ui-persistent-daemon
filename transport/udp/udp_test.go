package udp

import (
	"a2ui/protocol"
	"encoding/json"
	"testing"
)

func TestUDPAllowsTelemetryOnly(t *testing.T) {
	tele := protocol.Envelope{V: 1, Session: "s", Kind: protocol.KindTelemetry, Seq: 5, Payload: json.RawMessage(`{"meter":0.5}`)}
	b, err := Encode(tele, 1200)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Decode(b, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != protocol.KindTelemetry || got.Seq != 5 || got.Session != "s" {
		t.Fatalf("got=%+v", got)
	}
	_, err = Encode(protocol.Envelope{V: 1, Session: "s", Kind: protocol.KindOperation, Seq: 6}, 1200)
	if err == nil || err.Code != "udp.forbidden_kind" {
		t.Fatalf("got %#v", err)
	}
}

func TestUDPRejectsOversizeAndRequiresSequence(t *testing.T) {
	env := protocol.Envelope{V: 1, Session: "s", Kind: protocol.KindTelemetry, Seq: 0, Payload: json.RawMessage(`{"x":1}`)}
	_, err := Encode(env, 1200)
	if err == nil || err.Code != "udp.invalid_sequence" {
		t.Fatalf("got %#v", err)
	}
	env.Seq = 1
	env.Payload = json.RawMessage(`{"data":"abcdefghijklmnopqrstuvwxyz"}`)
	_, err = Encode(env, 32)
	if err == nil || err.Code != "resource.datagram_too_large" {
		t.Fatalf("got %#v", err)
	}
}

func TestUDPRejectsWrongEnvelopeVersionInsteadOfNormalizingIt(t *testing.T) {
	env := protocol.Envelope{V: 2, Session: "s", Kind: protocol.KindTelemetry, Seq: 1, Payload: json.RawMessage(`{"x":1}`)}
	_, err := Encode(env, 1200)
	if err == nil || err.Code != "protocol.version_mismatch" {
		t.Fatalf("got %#v", err)
	}
}

func TestUDPDecodeRejectsUnknownAndDuplicateHeaderFields(t *testing.T) {
	cases := []struct {
		raw  string
		code string
	}{
		{`{"magic":"A2UI","v":1,"session":"s","channel":"telemetry","seq":1,"payload":{},"wat":1}`, "wire.unknown_field"},
		{`{"magic":"A2UI","v":1,"session":"s","channel":"telemetry","seq":1,"seq":2,"payload":{}}`, "wire.duplicate_key"},
	}
	for _, tc := range cases {
		_, err := Decode([]byte(tc.raw), 1200)
		if err == nil || err.Code != tc.code {
			t.Fatalf("raw=%s got %#v", tc.raw, err)
		}
	}
}

func TestUDPTelemetryPayloadMustBeJSONObject(t *testing.T) {
	bad := protocol.Envelope{V: protocol.Version, Session: "s", Kind: protocol.KindTelemetry, Seq: 1, Payload: json.RawMessage(`[]`)}
	if _, err := Encode(bad, 1200); err == nil {
		t.Fatal("Encode accepted non-object telemetry payload")
	}

	raw := []byte(`{"magic":"A2UI","v":1,"session":"s","channel":"telemetry","seq":1,"payload":[]}`)
	if _, err := Decode(raw, 1200); err == nil {
		t.Fatal("Decode accepted non-object telemetry payload")
	}
}
