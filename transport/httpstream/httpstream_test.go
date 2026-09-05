package httpstream

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"a2ui/protocol"
	"a2ui/wire"
)

func TestHandlerPreservesReliableRecordOrder(t *testing.T) {
	h := NewHandler(4096, func(env protocol.Envelope) (protocol.Envelope, *protocol.Error) { return env, nil })
	body := strings.Join([]string{
		`{"v":1,"session":"s","kind":"operation","seq":1,"payload":{"v":1,"seq":1,"op":"commit"}}`,
		`{"v":1,"session":"s","kind":"operation","seq":2,"payload":{"v":1,"seq":2,"op":"commit"}}`,
		"",
	}, "\n")
	req := httptest.NewRequest(http.MethodPost, "/a2ui", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	lines := bytes.Split(bytes.TrimSpace(rec.Body.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("lines=%q", lines)
	}
	var a, b protocol.Envelope
	_ = json.Unmarshal(lines[0], &a)
	_ = json.Unmarshal(lines[1], &b)
	if a.Seq != 1 || b.Seq != 2 {
		t.Fatalf("order %d %d", a.Seq, b.Seq)
	}
}

func TestHandlerRejectsNonPostAndMalformed(t *testing.T) {
	h := NewHandler(128, func(env protocol.Envelope) (protocol.Envelope, *protocol.Error) { return env, nil })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatal(rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad}\n")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestHandlerFlushesEachAcceptedRecord(t *testing.T) {
	h := NewHandler(4096, func(env protocol.Envelope) (protocol.Envelope, *protocol.Error) { return env, nil })
	body := "{\"v\":1,\"session\":\"s\",\"kind\":\"telemetry\",\"seq\":1,\"payload\":{}}\n" +
		"{\"v\":1,\"session\":\"s\",\"kind\":\"telemetry\",\"seq\":2,\"payload\":{}}\n"
	w := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if w.flushes != 2 {
		t.Fatalf("expected one flush per record, got %d", w.flushes)
	}
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func (f *flushRecorder) Flush() { f.flushes++; f.ResponseRecorder.Flush() }

func TestHandlerRunsOverHTTP2WithStandardLibraryTLS(t *testing.T) {
	h := NewHandler(4096, func(env protocol.Envelope) (protocol.Envelope, *protocol.Error) { return env, nil })
	srv := httptest.NewUnstartedServer(h)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	resp, err := srv.Client().Post(srv.URL, "application/x-ndjson", strings.NewReader("{\"v\":1,\"session\":\"s\",\"kind\":\"telemetry\",\"seq\":1,\"payload\":{}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("expected HTTP/2, got %s", resp.Proto)
	}
}

func TestHandlerSurfacesMidstreamProtocolErrorAsA2UIEvent(t *testing.T) {
	h := NewHandler(4096, func(env protocol.Envelope) (protocol.Envelope, *protocol.Error) { return env, nil })
	body := "{\"v\":1,\"session\":\"s\",\"kind\":\"telemetry\",\"seq\":1,\"payload\":{}}\n{bad}\n"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	lines := bytes.Split(bytes.TrimSpace(rec.Body.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected response + error envelope, got %q", lines)
	}
	env, perr := wire.DecodeRecord(lines[1])
	if perr != nil {
		t.Fatalf("handler emitted non-conforming A2UI record: %v line=%s", perr, lines[1])
	}
	if env.Kind != protocol.KindEvent {
		t.Fatalf("kind=%q line=%s", env.Kind, lines[1])
	}
	var ev protocol.Event
	if err := json.Unmarshal(env.Payload, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Ev != "error" || ev.Code != "wire.malformed" {
		t.Fatalf("event=%+v", ev)
	}
}

func TestHandlerDoesNotReturn200ForInvalidFirstOutboundEnvelope(t *testing.T) {
	h := NewHandler(4096, func(protocol.Envelope) (protocol.Envelope, *protocol.Error) {
		return protocol.Envelope{
			V:       1,
			Session: "s",
			Kind:    protocol.KindOperation,
			Seq:     2,
			Payload: json.RawMessage(`{"v":1,"seq":1,"op":"commit"}`),
		}, nil
	})
	rec := httptest.NewRecorder()
	body := "{\"v\":1,\"session\":\"s\",\"kind\":\"telemetry\",\"seq\":1,\"payload\":{}}\n"
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("invalid server envelope returned status=%d body=%s", rec.Code, rec.Body.String())
	}
}
