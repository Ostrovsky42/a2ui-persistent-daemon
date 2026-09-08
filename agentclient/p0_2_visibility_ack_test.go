//go:build unix

package agentclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"a2ui/daemon"
	"a2ui/ipc"
	"a2ui/protocol"
)

func TestP02PublishDoesNotReturnVisibleBeforeExactRendererAck(t *testing.T) {
	d := daemon.New("p0-2-visible", protocol.DefaultLimits(), nil)
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
	if err := w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindHello, Client: "p0-2-visible-test"}); err != nil {
		t.Fatal(err)
	}
	if msg, perr, err := r.Next(); err != nil || perr != nil || msg.Kind != ipc.KindHelloAck {
		t.Fatalf("hello ack: msg=%+v perr=%v err=%v", msg, perr, err)
	}
	if msg, perr, err := r.Next(); err != nil || perr != nil || msg.Kind != ipc.KindSnapshot {
		t.Fatalf("initial snapshot: msg=%+v perr=%v err=%v", msg, perr, err)
	}

	httpServer := httptest.NewServer(d)
	defer httpServer.Close()
	client := New(httpServer.URL, "p0-2-visible", httpServer.Client())

	publishDone := make(chan error, 1)
	go func() {
		publishDone <- client.Publish(context.Background(), []protocol.Operation{
			{Op: protocol.OpUpsert, ID: "services", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"services"}`)},
			{Op: protocol.OpCommit, Frame: "services-visible"},
		})
	}()

	msg, perr, err := r.Next()
	if err != nil || perr != nil || msg.Kind != ipc.KindSnapshot || msg.Snapshot == nil {
		t.Fatalf("published snapshot: msg=%+v perr=%v err=%v", msg, perr, err)
	}
	generation := msg.Snapshot.Presentation.PublicationGeneration
	if generation == 0 || !msg.Snapshot.Presentation.PublicationPending {
		t.Fatalf("snapshot publication state = %+v", msg.Snapshot.Presentation)
	}

	select {
	case err := <-publishDone:
		t.Fatalf("publish returned before exact renderer ACK: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	if err := w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindFramePublished, PublicationGeneration: generation}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-publishDone:
		if err != nil {
			t.Fatalf("publish after ACK: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("publish did not complete after exact renderer ACK")
	}
}
