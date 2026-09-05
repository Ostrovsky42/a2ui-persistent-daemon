//go:build unix

package daemon

import (
	"encoding/json"
	"testing"
	"time"

	"a2ui/ipc"
	"a2ui/protocol"
)

func agentEnvelope(t *testing.T, d *Daemon, env protocol.Envelope) *protocol.Envelope {
	t.Helper()
	resp, perr := d.HandleEnvelope(env)
	if perr != nil {
		t.Fatalf("agent envelope %s: %v", env.Kind, perr)
	}
	return resp
}

func TestAgentEnvelopeMutationsPushLatestSnapshotWithoutPublishing(t *testing.T) {
	d := New("agent-session", protocol.DefaultLimits(), nil)
	path, _ := startTestDaemon(t, d)
	client := dialTestClient(t, path)
	defer client.conn.Close()
	_, _ = client.hello(t)

	helloPayload, _ := json.Marshal(protocol.Hello{Versions: []int{protocol.Version}, Features: []string{"commit-barrier"}})
	resp := agentEnvelope(t, d, protocol.Envelope{V: 1, Session: "agent-session", Kind: protocol.KindHello, Payload: helloPayload})
	if resp == nil || resp.Kind != protocol.KindHelloAck {
		t.Fatalf("hello response=%+v", resp)
	}

	op := protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "txt", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"hello"}`)}
	opPayload, _ := json.Marshal(op)
	agentEnvelope(t, d, protocol.Envelope{V: 1, Session: "agent-session", Kind: protocol.KindOperation, Seq: 1, Payload: opPayload})

	_ = client.conn.SetReadDeadline(time.Now().Add(time.Second))
	pushed, perr, err := client.r.Next()
	_ = client.conn.SetReadDeadline(time.Time{})
	if err != nil || perr != nil {
		t.Fatalf("push perr=%v err=%v", perr, err)
	}
	if pushed.Kind != ipc.KindSnapshot || pushed.Snapshot == nil || pushed.Snapshot.Presentation.Document.Nodes["txt"].ID != "txt" {
		t.Fatalf("pushed snapshot=%+v", pushed)
	}
	if !pushed.Snapshot.Presentation.PublicationPending {
		t.Fatal("agent mutation push was not marked publication pending")
	}
	if _, ok := d.Engine.NextEvent(); ok {
		t.Fatal("snapshot push emitted event before commit/ack")
	}
}

func TestAgentCommitWaitsForRemoteFrameAckBeforeCommittedEvent(t *testing.T) {
	d := New("agent-session", protocol.DefaultLimits(), nil)
	path, _ := startTestDaemon(t, d)
	client := dialTestClient(t, path)
	defer client.conn.Close()
	_, _ = client.hello(t)

	helloPayload, _ := json.Marshal(protocol.Hello{Versions: []int{protocol.Version}})
	agentEnvelope(t, d, protocol.Envelope{V: 1, Session: "agent-session", Kind: protocol.KindHello, Payload: helloPayload})
	op := protocol.Operation{V: 1, Seq: 1, Op: protocol.OpCommit, Frame: "remote-frame"}
	payload, _ := json.Marshal(op)
	agentEnvelope(t, d, protocol.Envelope{V: 1, Session: "agent-session", Kind: protocol.KindOperation, Seq: 1, Payload: payload})

	_ = client.conn.SetReadDeadline(time.Now().Add(time.Second))
	snap, perr, err := client.r.Next()
	_ = client.conn.SetReadDeadline(time.Time{})
	if err != nil || perr != nil || snap.Kind != ipc.KindSnapshot {
		t.Fatalf("commit push=%+v perr=%v err=%v", snap, perr, err)
	}
	if _, ok := d.Engine.NextEvent(); ok {
		t.Fatal("commit acknowledged before remote frame ACK")
	}
	gen := snap.Snapshot.Presentation.PublicationGeneration
	if err := client.w.Write(ipc.Message{V: 1, Kind: ipc.KindFramePublished, PublicationGeneration: gen}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		events := d.DrainEvents()
		if len(events) > 0 {
			if events[0].Ev != "committed" || events[0].Frame != "remote-frame" {
				t.Fatalf("events=%+v", events)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("committed event not delivered after remote frame ACK")
}
