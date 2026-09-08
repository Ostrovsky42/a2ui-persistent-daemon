//go:build unix

package ipc_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/daemon"
	"github.com/Ostrovsky42/agent-interaction-runtime/ipc"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func startDaemonForClientTest(t *testing.T, d *daemon.Daemon) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a2ui.sock")
	ln, perr := ipc.ListenUnix(path)
	if perr != nil {
		t.Fatal(perr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = d.Serve(ctx, ln) }()
	t.Cleanup(func() { cancel(); _ = ln.Close() })
	return path
}

func negotiateAgent(t *testing.T, d *daemon.Daemon) {
	t.Helper()
	payload, _ := json.Marshal(protocol.Hello{Versions: []int{protocol.Version}})
	if _, perr := d.HandleEnvelope(protocol.Envelope{V: 1, Session: d.Session.ID(), Kind: protocol.KindHello, Payload: payload}); perr != nil {
		t.Fatal(perr)
	}
}

func agentOp(t *testing.T, d *daemon.Daemon, seq uint64, op protocol.Operation) {
	t.Helper()
	op.V, op.Seq = protocol.Version, int64(seq)
	payload, _ := json.Marshal(op)
	if _, perr := d.HandleEnvelope(protocol.Envelope{V: 1, Session: d.Session.ID(), Kind: protocol.KindOperation, Seq: seq, Payload: payload}); perr != nil {
		t.Fatal(perr)
	}
}

func TestClientCachesSnapshotsAndExecutesRemoteSemanticInteractions(t *testing.T) {
	d := daemon.New("session", protocol.DefaultLimits(), nil)
	path := startDaemonForClientTest(t, d)
	client, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
	if perr != nil {
		t.Fatal(perr)
	}
	defer client.Close()
	if client.Snapshot().Document.Nodes["root"].ID != "root" {
		t.Fatal("initial snapshot missing root")
	}

	negotiateAgent(t, d)
	agentOp(t, d, 1, protocol.Operation{Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent"}`)})
	select {
	case <-client.Updates():
	case <-time.After(time.Second):
		t.Fatal("client did not receive daemon snapshot push")
	}
	if client.Snapshot().InputValues["in"] != "agent" {
		t.Fatalf("snapshot=%+v", client.Snapshot())
	}

	if err := client.Focus("in"); err != nil {
		t.Fatal(err)
	}
	if err := client.SetInput("in", "typed"); err != nil {
		t.Fatal(err)
	}
	snap := client.Snapshot()
	if snap.FocusedID != "in" || snap.InputValues["in"] != "typed" {
		t.Fatalf("remote state=%+v", snap)
	}
}

func TestClientDisconnectDoesNotDestroyDaemonSemanticState(t *testing.T) {
	d := daemon.New("session", protocol.DefaultLimits(), nil)
	path := startDaemonForClientTest(t, d)
	negotiateAgent(t, d)
	agentOp(t, d, 1, protocol.Operation{Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent"}`)})

	c1, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
	if perr != nil {
		t.Fatal(perr)
	}
	if err := c1.Focus("in"); err != nil {
		t.Fatal(err)
	}
	if err := c1.SetInput("in", "persistent"); err != nil {
		t.Fatal(err)
	}
	_ = c1.Close()

	deadline := time.Now().Add(time.Second)
	var c2 *ipc.Client
	for time.Now().Before(deadline) {
		var err *ipc.Error
		c2, err = ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
		if err == nil {
			break
		}
		if err.Code != "ipc.client_busy" {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if c2 == nil {
		t.Fatal("could not reattach")
	}
	defer c2.Close()
	snap := c2.Snapshot()
	if snap.FocusedID != "in" || snap.InputValues["in"] != "persistent" {
		t.Fatalf("reattach lost semantic state: %+v", snap)
	}
}

func TestStalePublicationErrorDoesNotMarkClientDisconnected(t *testing.T) {
	d := daemon.New("session", protocol.DefaultLimits(), nil)
	path := startDaemonForClientTest(t, d)
	negotiateAgent(t, d)
	agentOp(t, d, 1, protocol.Operation{Op: protocol.OpUpsert, ID: "txt", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"one"}`)})
	client, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
	if perr != nil {
		t.Fatal(perr)
	}
	defer client.Close()
	old := client.Snapshot().PublicationGeneration

	agentOp(t, d, 2, protocol.Operation{Op: protocol.OpProps, ID: "txt", Props: json.RawMessage(`{"text":"two"}`)})
	if err := client.AcknowledgePublication(old); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for client.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if client.Err() == nil {
		t.Fatal("expected stale publication diagnostic")
	}
	if err := client.ConnectionError(); err != nil {
		t.Fatalf("stale publication is not a connection failure: %v", err)
	}
	err := client.SetInput("missing", "x")
	if err == nil {
		t.Fatal("expected semantic missing-node response")
	}
	if strings.Contains(err.Error(), "ipc.connection_closed") {
		t.Fatalf("connection was incorrectly treated as closed: %v", err)
	}
}
