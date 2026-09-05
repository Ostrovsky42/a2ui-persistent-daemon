package wire

import (
	"errors"
	"io"
	"strings"
	"testing"

	"a2ui/protocol"
)

func TestLegacyOperationNormalizesToEnvelope(t *testing.T) {
	r := NewNDJSONReader(strings.NewReader("{\"v\":1,\"seq\":7,\"op\":\"commit\",\"frame\":\"x\"}\n"), 1024)
	env, perr, err := r.Next()
	if err != nil || perr != nil {
		t.Fatalf("err=%v perr=%v", err, perr)
	}
	if env.Kind != protocol.KindOperation || env.Seq != 7 {
		t.Fatalf("env=%+v", env)
	}
}

func TestMalformedLineIsRecoverableAndNextRecordStillReads(t *testing.T) {
	r := NewNDJSONReader(strings.NewReader("{bad}\n{\"v\":1,\"seq\":1,\"op\":\"commit\"}\n"), 1024)
	_, perr, err := r.Next()
	if err != nil || perr == nil || perr.Code != "wire.malformed" {
		t.Fatalf("err=%v perr=%#v", err, perr)
	}
	env, perr, err := r.Next()
	if err != nil || perr != nil || env.Kind != protocol.KindOperation {
		t.Fatalf("env=%+v err=%v perr=%v", env, err, perr)
	}
}

func TestOversizedRecordRejectedWithoutLosingNextRecord(t *testing.T) {
	input := strings.Repeat("x", 33) + "\n{\"v\":1,\"seq\":1,\"op\":\"commit\"}\n"
	r := NewNDJSONReader(strings.NewReader(input), 32)
	_, perr, err := r.Next()
	if err != nil || perr == nil || perr.Code != "resource.record_too_large" {
		t.Fatalf("err=%v perr=%#v", err, perr)
	}
	env, perr, err := r.Next()
	if err != nil || perr != nil || env.Kind != protocol.KindOperation {
		t.Fatalf("env=%+v err=%v perr=%v", env, err, perr)
	}
}

func TestCleanEOFIsDistinctFromReaderError(t *testing.T) {
	r := NewNDJSONReader(strings.NewReader(""), 32)
	_, _, err := r.Next()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("clean EOF=%v", err)
	}
	br := &failingReader{}
	r = NewNDJSONReader(br, 32)
	_, _, err = r.Next()
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected transport error, got %v", err)
	}
}

type failingReader struct{}

func (*failingReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestRecordLimitCountsJSONBytesNotNewlineFraming(t *testing.T) {
	line := `{"v":1,"seq":1,"op":"commit"}`
	r := NewNDJSONReader(strings.NewReader(line+"\n"), len(line))
	env, perr, err := r.Next()
	if err != nil || perr != nil || env.Kind != protocol.KindOperation {
		t.Fatalf("exact-size record rejected: env=%+v perr=%v err=%v", env, perr, err)
	}
}

func TestWireRejectsUnknownTopLevelFields(t *testing.T) {
	cases := []string{
		`{"v":1,"seq":1,"op":"commit","wat":1}`,
		`{"v":1,"session":"s","kind":"telemetry","seq":1,"payload":{},"wat":1}`,
	}
	for _, line := range cases {
		_, perr := DecodeRecord([]byte(line))
		if perr == nil || perr.Code != "wire.unknown_field" {
			t.Fatalf("line=%s got %#v", line, perr)
		}
	}
}

func TestWireRejectsDuplicateJSONKeysAtAnyDepth(t *testing.T) {
	cases := []string{
		`{"v":1,"seq":1,"seq":2,"op":"commit"}`,
		`{"v":1,"seq":1,"op":"upsert","id":"x","type":"text","props":{"text":"a","text":"b"}}`,
	}
	for _, line := range cases {
		_, perr := DecodeRecord([]byte(line))
		if perr == nil || perr.Code != "wire.duplicate_key" {
			t.Fatalf("line=%s got %#v", line, perr)
		}
	}
}

func TestDirectEventNormalizesToEnvelope(t *testing.T) {
	line := `{"v":1,"seq":9,"ev":"error","code":"schema.invalid_props","msg":"bad","row":0}`
	env, perr := DecodeRecord([]byte(line))
	if perr != nil {
		t.Fatal(perr)
	}
	if env.Kind != protocol.KindEvent || env.V != protocol.Version || env.Seq != 9 {
		t.Fatalf("env=%+v", env)
	}
	if string(env.Payload) != line {
		t.Fatalf("payload=%s", env.Payload)
	}
}

func TestLegacyOperationRequiresVersionAndSequenceFields(t *testing.T) {
	cases := []string{
		`{"v":1,"op":"commit"}`,
		`{"seq":1,"op":"commit"}`,
	}
	for _, line := range cases {
		_, perr := DecodeRecord([]byte(line))
		if perr == nil || perr.Code != "wire.missing_field" {
			t.Fatalf("line=%s got %#v", line, perr)
		}
	}
}

func TestDecodeEmptyRecordHasSpecificError(t *testing.T) {
	_, perr := DecodeRecord([]byte("   \t"))
	if perr == nil || perr.Code != "wire.empty" {
		t.Fatalf("got %#v", perr)
	}
}

func TestDirectNonSelectEventDoesNotRequireRow(t *testing.T) {
	line := `{"v":1,"seq":9,"ev":"error","code":"schema.invalid_props","msg":"bad"}`
	env, perr := DecodeRecord([]byte(line))
	if perr != nil {
		t.Fatalf("got %#v", perr)
	}
	if env.Kind != protocol.KindEvent || env.Seq != 9 {
		t.Fatalf("env=%+v", env)
	}
}

func TestOperationEnvelopeValidatesNestedPayloadAndSequence(t *testing.T) {
	cases := []struct {
		line string
		code string
	}{
		{`{"v":1,"session":"s","kind":"operation","seq":2,"payload":{"v":1,"seq":1,"op":"commit"}}`, "protocol.sequence_mismatch"},
		{`{"v":1,"session":"s","kind":"operation","seq":1,"payload":{"v":1,"seq":1,"op":"commit","wat":1}}`, "wire.unknown_field"},
		{`{"v":1,"session":"s","kind":"operation","payload":{"v":1,"seq":1,"op":"commit"}}`, "wire.missing_field"},
		{`{"v":1,"session":"s","kind":"operation","seq":1}`, "wire.missing_field"},
	}
	for _, tc := range cases {
		_, perr := DecodeRecord([]byte(tc.line))
		if perr == nil || perr.Code != tc.code {
			t.Fatalf("line=%s got %#v", tc.line, perr)
		}
	}
}

func TestEventEnvelopeValidatesNestedEventAndSequence(t *testing.T) {
	cases := []struct {
		line string
		code string
	}{
		{`{"v":1,"session":"s","kind":"event","seq":2,"payload":{"v":1,"seq":1,"ev":"error","code":"x"}}`, "protocol.sequence_mismatch"},
		{`{"v":1,"session":"s","kind":"event","seq":1,"payload":{"v":1,"seq":1,"ev":"error","wat":1}}`, "wire.unknown_field"},
		{`{"v":1,"session":"s","kind":"event","payload":{"v":1,"seq":1,"ev":"error"}}`, "wire.missing_field"},
	}
	for _, tc := range cases {
		_, perr := DecodeRecord([]byte(tc.line))
		if perr == nil || perr.Code != tc.code {
			t.Fatalf("line=%s got %#v", tc.line, perr)
		}
	}
}

func TestHelloEnvelopeRequiresStrictHelloPayload(t *testing.T) {
	cases := []struct {
		line string
		code string
	}{
		{`{"v":1,"session":"s","kind":"hello","payload":{"versions":[1],"wat":1}}`, "wire.unknown_field"},
		{`{"v":1,"session":"s","kind":"hello","payload":{}}`, "schema.missing_field"},
		{`{"v":1,"session":"s","kind":"hello"}`, "wire.missing_field"},
	}
	for _, tc := range cases {
		_, perr := DecodeRecord([]byte(tc.line))
		if perr == nil || perr.Code != tc.code {
			t.Fatalf("line=%s got %#v", tc.line, perr)
		}
	}
}

func TestHelloAckEnvelopeRejectsMissingFiniteLimits(t *testing.T) {
	line := `{"v":1,"session":"s","kind":"hello_ack","payload":{"version":1,"features":[],"components":["box"]}}`
	_, perr := DecodeRecord([]byte(line))
	if perr == nil || perr.Code != "schema.invalid_limits" {
		t.Fatalf("got %#v", perr)
	}
}

func TestHandshakeEnvelopeValidationMatchesSchema(t *testing.T) {
	cases := []struct {
		line string
		code string
	}{
		{`{"v":1,"session":"s","kind":"hello","payload":{"versions":[1,1]}}`, "schema.invalid_hello"},
		{`{"v":1,"session":"s","kind":"hello_ack","payload":{"version":1,"features":["x","x"],"components":["box"],"limits":{"max_message_bytes":1,"max_nodes":1,"max_depth":1,"max_children":1,"max_text_bytes_per_node":1,"max_total_text_bytes":1,"max_table_rows":1,"max_table_columns":1,"max_pending_events":1,"max_pending_mutations":1,"max_inflight_actions":1,"max_datagram_bytes":1}}}`, "schema.invalid_hello_ack"},
	}
	for _, tc := range cases {
		_, perr := DecodeRecord([]byte(tc.line))
		if perr == nil || perr.Code != tc.code {
			t.Fatalf("line=%s got %#v", tc.line, perr)
		}
	}
}

func TestEncodeEnvelopeRejectsInvalidNestedPayload(t *testing.T) {
	env := protocol.Envelope{
		V:       protocol.Version,
		Session: "s",
		Kind:    protocol.KindOperation,
		Seq:     2,
		Payload: []byte(`{"v":1,"seq":1,"op":"commit"}`),
	}
	if _, err := EncodeEnvelope(env); err == nil {
		t.Fatal("invalid sequence-mismatched envelope was serialized")
	}
}

func TestTelemetryEnvelopeRequiresObjectPayload(t *testing.T) {
	for _, line := range []string{
		`{"v":1,"session":"s","kind":"telemetry","seq":1,"payload":null}`,
		`{"v":1,"session":"s","kind":"telemetry","seq":1,"payload":[]}`,
	} {
		if _, perr := DecodeRecord([]byte(line)); perr == nil {
			t.Fatalf("accepted non-object telemetry payload: %s", line)
		}
	}
}
