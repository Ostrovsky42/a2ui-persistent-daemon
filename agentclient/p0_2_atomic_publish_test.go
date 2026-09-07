//go:build unix

package agentclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"a2ui/daemon"
	"a2ui/ipc"
	"a2ui/protocol"
)

func TestP02OneAgentPublishProducesOneIPCSnapshotTransition(t *testing.T) {
	d := daemon.New("p0-2-atomic", protocol.DefaultLimits(), nil)
	if perr := d.Engine.Apply(protocol.Operation{V: protocol.Version, Seq: 1, Op: protocol.OpUpsert, ID: "old-detail", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"screen A"}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.Publish(); perr != nil {
		t.Fatal(perr)
	}

	sock := filepath.Join(t.TempDir(), "a2ui.sock")
	ln, ierr := ipc.ListenUnix(sock)
	if ierr != nil {
		t.Fatal(ierr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = d.Serve(ctx, ln) }()

	conn, err := net.DialTimeout("unix", sock, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	r := ipc.NewReader(conn, ipc.DefaultMaxMessageBytes)
	w := ipc.NewWriter(conn)
	if err := w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindHello, Client: "p0-2-test"}); err != nil {
		t.Fatal(err)
	}
	if msg, perr, err := r.Next(); err != nil || perr != nil || msg.Kind != ipc.KindHelloAck {
		t.Fatalf("hello ack: msg=%+v perr=%v err=%v", msg, perr, err)
	}
	initial, perr, err := r.Next()
	if err != nil || perr != nil || initial.Kind != ipc.KindSnapshot || initial.Snapshot == nil {
		t.Fatalf("initial snapshot: msg=%+v perr=%v err=%v", initial, perr, err)
	}
	if _, ok := initial.Snapshot.Presentation.Document.Nodes["old-detail"]; !ok {
		t.Fatalf("initial snapshot is not screen A: %+v", initial.Snapshot.Presentation.Document)
	}

	// Delay completion of each HTTP operation slightly so the real IPC writer
	// has time to expose every inherited per-mutation snapshot deterministically.
	httpServer := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		d.ServeHTTP(rw, req)
		time.Sleep(15 * time.Millisecond)
	}))
	defer httpServer.Close()

	client := New(httpServer.URL, "p0-2-atomic", httpServer.Client())
	ops := []protocol.Operation{
		{Op: protocol.OpRemove, ID: "old-detail"},
		{Op: protocol.OpUpsert, ID: "header", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"Services"}`)},
		{Op: protocol.OpUpsert, ID: "services", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"rows":[["api"]],"row_ids":["api"]}`)},
		{Op: protocol.OpFocus, ID: "services"},
		{Op: protocol.OpCommit, Frame: "services-B"},
	}
	if err := client.Publish(context.Background(), ops); err != nil {
		t.Fatalf("publish screen B: %v", err)
	}

	var snapshots []ipc.Message
	for {
		_ = conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
		msg, perr, err := r.Next()
		if ne, ok := err.(net.Error); ok && ne.Timeout() {
			break
		}
		if err != nil || perr != nil {
			t.Fatalf("read publish snapshot: msg=%+v perr=%v err=%v", msg, perr, err)
		}
		if msg.Kind == ipc.KindSnapshot {
			snapshots = append(snapshots, msg)
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	if len(snapshots) != 1 {
		t.Fatalf("one a2ui publish exposed %d IPC snapshots; want exactly one atomic B transition", len(snapshots))
	}
	final := snapshots[0].Snapshot.Presentation
	if _, ok := final.Document.Nodes["old-detail"]; ok {
		t.Fatal("final screen still contains old detail")
	}
	if _, ok := final.Document.Nodes["header"]; !ok {
		t.Fatal("final screen missing header")
	}
	if _, ok := final.Document.Nodes["services"]; !ok || final.FocusedID != "services" {
		t.Fatalf("final services/focus mismatch: %+v", final)
	}
}
