//go:build unix

package agentclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"a2ui/daemon"
	"a2ui/ipc"
	"a2ui/protocol"
)

func TestP02InvalidPublishBatchRollsBackSemanticRuntimeAndSequenceState(t *testing.T) {
	d := daemon.New("p0-2-failure", protocol.DefaultLimits(), nil)
	initialOps := []protocol.Operation{
		{V: protocol.Version, Seq: 1, Op: protocol.OpUpsert, ID: "query", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent"}`)},
		{V: protocol.Version, Seq: 2, Op: protocol.OpUpsert, ID: "jobs", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)},
		{V: protocol.Version, Seq: 3, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Reload","action":"reload"}]}`)},
	}
	for _, op := range initialOps {
		if perr := d.Engine.Apply(op); perr != nil {
			t.Fatal(perr)
		}
	}
	if perr := d.Engine.SetInput("query", "local-value"); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.SetTableSelection("jobs", 1); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.Focus("query"); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.Publish(); perr != nil {
		t.Fatal(perr)
	}
	before := d.Engine.PresentationSnapshot()

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
	if err := w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindHello, Client: "p0-2-failure-test"}); err != nil {
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
	client := New(httpServer.URL, "p0-2-failure", httpServer.Client())
	failed := []protocol.Operation{
		{Op: protocol.OpRemove, ID: "jobs"},
		{Op: protocol.OpFocus, ID: "missing-focus-target"},
		{Op: protocol.OpUpsert, ID: "must-not-appear", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"partial"}`)},
	}
	if err := client.Publish(context.Background(), failed); err == nil {
		t.Fatal("invalid batch unexpectedly succeeded")
	}

	after := d.Engine.PresentationSnapshot()
	if !reflect.DeepEqual(after, before) {
		t.Errorf("failed batch mutated semantic/runtime presentation\nbefore=%+v\nafter=%+v", before, after)
	}

	_ = conn.SetReadDeadline(time.Now().Add(80 * time.Millisecond))
	msg, perr, readErr := r.Next()
	if ne, ok := readErr.(net.Error); ok && ne.Timeout() {
		// Required: failed batch produced no renderer snapshot.
	} else if readErr != nil || perr != nil {
		t.Fatalf("unexpected IPC read after failed batch: msg=%+v perr=%v err=%v", msg, perr, readErr)
	} else if msg.Kind == ipc.KindSnapshot {
		t.Errorf("failed batch emitted renderer snapshot: %+v", msg.Snapshot)
	}
	_ = conn.SetReadDeadline(time.Time{})

	if err := client.Publish(context.Background(), []protocol.Operation{
		{Op: protocol.OpUpsert, ID: "recovery", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"ok"}`)},
		{Op: protocol.OpCommit, Frame: "recovery"},
	}); err != nil {
		t.Errorf("valid publish after failed batch did not continue predictably: %v", err)
	}
}
