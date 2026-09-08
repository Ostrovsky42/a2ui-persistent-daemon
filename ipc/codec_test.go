package ipc

import (
	"bytes"
	"errors"
	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestCodecRoundTripsCoreMessages(t *testing.T) {
	cases := []Message{
		{V: Version, Kind: KindHello, Client: "bubbletea"},
		{V: Version, Kind: KindHelloAck, ClientID: "client-1", Interactive: true},
		{V: Version, Kind: KindInteraction, Interaction: &Interaction{Type: InteractionInputSet, ID: "cmd", Value: "hello"}},
		{V: Version, Kind: KindFramePublished, PublicationGeneration: 7},
		{V: Version, Kind: KindDetach},
	}
	for _, want := range cases {
		raw, err := EncodeMessage(want)
		if err != nil {
			t.Fatalf("encode %s: %v", want.Kind, err)
		}
		got, perr := DecodeMessage(raw)
		if perr != nil {
			t.Fatalf("decode %s: %v", want.Kind, perr)
		}
		if got.Kind != want.Kind || got.V != want.V || got.Client != want.Client || got.ClientID != want.ClientID || got.Interactive != want.Interactive || got.PublicationGeneration != want.PublicationGeneration {
			t.Fatalf("roundtrip mismatch for %s: got %+v want %+v", want.Kind, got, want)
		}
		if want.Interaction != nil {
			if got.Interaction == nil || !reflect.DeepEqual(*got.Interaction, *want.Interaction) {
				t.Fatalf("interaction mismatch: got %+v want %+v", got.Interaction, want.Interaction)
			}
		}
	}
}

func TestSnapshotMessageRoundTrip(t *testing.T) {
	eng := engine.New(protocol.DefaultLimits(), 8, engine.NewNoopActions())
	snap := eng.PresentationSnapshot()
	want := Message{V: Version, Kind: KindSnapshot, Snapshot: &Snapshot{Presentation: snap}}
	raw, err := EncodeMessage(want)
	if err != nil {
		t.Fatal(err)
	}
	got, perr := DecodeMessage(raw)
	if perr != nil {
		t.Fatal(perr)
	}
	if got.Snapshot == nil || got.Snapshot.Presentation.Document.Nodes["root"].ID != "root" {
		t.Fatalf("snapshot roundtrip lost document: %+v", got.Snapshot)
	}
	if got.Snapshot.Presentation.PublicationPending {
		t.Fatal("new engine snapshot unexpectedly pending")
	}
}

func TestDecodeMessageRejectsUnknownFieldAndKind(t *testing.T) {
	if _, perr := DecodeMessage([]byte(`{"v":1,"kind":"hello","client":"bubbletea","surprise":1}`)); perr == nil || perr.Code != "ipc.invalid_message" {
		t.Fatalf("expected ipc.invalid_message for unknown field, got %+v", perr)
	}
	if _, perr := DecodeMessage([]byte(`{"v":1,"kind":"wat"}`)); perr == nil || perr.Code != "ipc.invalid_message" {
		t.Fatalf("expected ipc.invalid_message for unknown kind, got %+v", perr)
	}
}

func TestDecodeMessageRejectsVersionMismatchAndInvalidInteraction(t *testing.T) {
	if _, perr := DecodeMessage([]byte(`{"v":2,"kind":"hello","client":"bubbletea"}`)); perr == nil || perr.Code != "ipc.version_mismatch" {
		t.Fatalf("expected version mismatch, got %+v", perr)
	}
	if _, perr := DecodeMessage([]byte(`{"v":1,"kind":"interaction","interaction":{"type":"focus"}}`)); perr == nil || perr.Code != "ipc.invalid_interaction" {
		t.Fatalf("expected invalid interaction, got %+v", perr)
	}
}

func TestReaderBoundsRecordsAndRecoversAfterOversize(t *testing.T) {
	input := strings.Repeat("x", 33) + "\n" + `{"v":1,"kind":"detach"}` + "\n"
	r := NewReader(strings.NewReader(input), 32)
	if _, perr, err := r.Next(); err != nil || perr == nil || perr.Code != "ipc.message_too_large" {
		t.Fatalf("expected oversize error, got msg err=%v perr=%+v", err, perr)
	}
	got, perr, err := r.Next()
	if err != nil || perr != nil || got.Kind != KindDetach {
		t.Fatalf("expected recovery to detach, got %+v perr=%+v err=%v", got, perr, err)
	}
	if _, _, err := r.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestWriterProducesSerializedNDJSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.Write(Message{V: Version, Kind: KindHello, Client: "bubbletea"}); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(Message{V: Version, Kind: KindDetach}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 records, got %d: %q", len(lines), buf.String())
	}
	for _, line := range lines {
		if _, perr := DecodeMessage([]byte(line)); perr != nil {
			t.Fatalf("invalid output line: %v", perr)
		}
	}
}

func TestDefaultIPCMessageBudgetCanCarryMaximumDocumentSnapshot(t *testing.T) {
	limits := protocol.DefaultLimits()
	if DefaultMaxMessageBytes <= limits.MaxDocumentBytes {
		t.Fatalf("IPC message budget=%d must exceed max retained Document bytes=%d to leave JSON snapshot overhead", DefaultMaxMessageBytes, limits.MaxDocumentBytes)
	}
}

func TestWriterSerializesConcurrentNDJSONRecords(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	const records = 128
	var wg sync.WaitGroup
	wg.Add(records)
	for i := 0; i < records; i++ {
		go func() {
			defer wg.Done()
			if err := w.Write(Message{V: Version, Kind: KindDetach}); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}
	wg.Wait()
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != records {
		t.Fatalf("record count=%d, want %d", len(lines), records)
	}
	for i, line := range lines {
		msg, perr := DecodeMessage([]byte(line))
		if perr != nil || msg.Kind != KindDetach {
			t.Fatalf("record %d interleaved/corrupt: msg=%+v err=%v raw=%q", i, msg, perr, line)
		}
	}
}

func FuzzDecodeMessage(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"v":1,"kind":"detach"}`),
		[]byte(`{"v":1,"kind":"hello","client":"bubbletea"}`),
		[]byte(`{"v":1,"kind":"interaction","interaction":{"type":"focus","id":"x"}}`),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		msg, perr := DecodeMessage(raw)
		if perr != nil {
			return
		}
		encoded, err := EncodeMessage(msg)
		if err != nil {
			t.Fatalf("accepted message could not be re-encoded: %v", err)
		}
		if _, perr := DecodeMessage(encoded); perr != nil {
			t.Fatalf("re-encoded accepted message rejected: %v", perr)
		}
	})
}
