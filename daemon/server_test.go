//go:build unix

package daemon

import (
	"context"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"a2ui/ipc"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
)

type testIPCConn struct {
	conn net.Conn
	r    *ipc.Reader
	w    *ipc.Writer
}

func dialTestClient(t *testing.T, path string) *testIPCConn {
	t.Helper()
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return &testIPCConn{conn: conn, r: ipc.NewReader(conn, ipc.DefaultMaxMessageBytes), w: ipc.NewWriter(conn)}
}

func (c *testIPCConn) hello(t *testing.T) (ipc.Message, ipc.Message) {
	t.Helper()
	if err := c.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindHello, Client: "bubbletea"}); err != nil {
		t.Fatal(err)
	}
	ack, perr, err := c.r.Next()
	if err != nil || perr != nil {
		t.Fatalf("hello ack: msg=%+v perr=%v err=%v", ack, perr, err)
	}
	snap, perr, err := c.r.Next()
	if err != nil || perr != nil {
		t.Fatalf("snapshot: msg=%+v perr=%v err=%v", snap, perr, err)
	}
	return ack, snap
}

func startTestDaemon(t *testing.T, d *Daemon) (string, context.CancelFunc) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a2ui.sock")
	ln, perr := ipc.ListenUnix(path)
	if perr != nil {
		t.Fatal(perr)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = d.Serve(ctx, ln) }()
	t.Cleanup(func() { cancel(); _ = ln.Close() })
	return path, cancel
}

func TestInitialAttachReceivesAtomicPersistentSnapshot(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent"}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.SetInput("in", "local"); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.Focus("in"); perr != nil {
		t.Fatal(perr)
	}
	path, _ := startTestDaemon(t, d)

	client := dialTestClient(t, path)
	defer client.conn.Close()
	ack, snapMsg := client.hello(t)
	if ack.Kind != ipc.KindHelloAck || ack.ClientID == "" || !ack.Interactive {
		t.Fatalf("bad ack %+v", ack)
	}
	if snapMsg.Kind != ipc.KindSnapshot || snapMsg.Snapshot == nil {
		t.Fatalf("bad snapshot %+v", snapMsg)
	}
	got := snapMsg.Snapshot.Presentation
	want := d.Engine.PresentationSnapshot()
	if got.Document.Revision != want.Document.Revision || got.FocusedID != "in" || got.InputValues["in"] != "local" {
		t.Fatalf("snapshot mismatch got=%+v want=%+v", got, want)
	}
}

func TestSingleInteractiveClientBusyThenLeaseReleasedOnDisconnect(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	path, _ := startTestDaemon(t, d)

	c1 := dialTestClient(t, path)
	ack, _ := c1.hello(t)
	if ack.ClientID == "" {
		t.Fatal("first client missing id")
	}

	c2 := dialTestClient(t, path)
	defer c2.conn.Close()
	if err := c2.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindHello, Client: "bubbletea"}); err != nil {
		t.Fatal(err)
	}
	busy, perr, err := c2.r.Next()
	if err != nil || perr != nil {
		t.Fatalf("busy response perr=%v err=%v", perr, err)
	}
	if busy.Kind != ipc.KindError || busy.Error == nil || busy.Error.Code != "ipc.client_busy" {
		t.Fatalf("second client response=%+v", busy)
	}

	_ = c1.conn.Close()
	deadline := time.Now().Add(time.Second)
	for d.lease.owner() != "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if d.lease.owner() != "" {
		t.Fatal("lease did not release after disconnect")
	}

	c3 := dialTestClient(t, path)
	defer c3.conn.Close()
	ack3, _ := c3.hello(t)
	if ack3.Kind != ipc.KindHelloAck {
		t.Fatalf("reattach failed: %+v", ack3)
	}
}

func sendInteraction(t *testing.T, c *testIPCConn, req string, in ipc.Interaction) ipc.Message {
	t.Helper()
	if err := c.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindInteraction, RequestID: req, Interaction: &in}); err != nil {
		t.Fatal(err)
	}
	msg, perr, err := c.r.Next()
	if err != nil || perr != nil {
		t.Fatalf("interaction %s response perr=%v err=%v", req, perr, err)
	}
	if msg.Kind == ipc.KindError {
		t.Fatalf("interaction %s error: %+v", req, msg.Error)
	}
	if msg.Kind != ipc.KindSnapshot || msg.RequestID != req || msg.Snapshot == nil {
		t.Fatalf("interaction %s response=%+v", req, msg)
	}
	return msg
}

func TestRemoteSemanticInteractionsMutateOnlyDaemonRuntime(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	ops := []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent","action":"submit"}`)},
		{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "jobs", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"action":"open_job","rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)},
	}
	for _, op := range ops {
		if perr := d.Engine.Apply(op); perr != nil {
			t.Fatal(perr)
		}
	}
	path, _ := startTestDaemon(t, d)
	c := dialTestClient(t, path)
	defer c.conn.Close()
	_, _ = c.hello(t)

	msg := sendInteraction(t, c, "focus-1", ipc.Interaction{Type: ipc.InteractionFocus, ID: "in"})
	if msg.Snapshot.Presentation.FocusedID != "in" {
		t.Fatalf("focus=%q", msg.Snapshot.Presentation.FocusedID)
	}
	msg = sendInteraction(t, c, "input-1", ipc.Interaction{Type: ipc.InteractionInputSet, ID: "in", Value: "local"})
	if msg.Snapshot.Presentation.InputValues["in"] != "local" {
		t.Fatalf("input=%q", msg.Snapshot.Presentation.InputValues["in"])
	}
	msg = sendInteraction(t, c, "table-1", ipc.Interaction{Type: ipc.InteractionTableMove, ID: "jobs", Delta: 1})
	if got := msg.Snapshot.Presentation.TableSelections["jobs"].RowID; got != "job-b" {
		t.Fatalf("selection=%q", got)
	}

	_ = sendInteraction(t, c, "activate-1", ipc.Interaction{Type: ipc.InteractionTableActivate, ID: "jobs"})
	ev, ok := d.Engine.NextEvent()
	if !ok || ev.Ev != "select" || ev.RowID != "job-b" {
		t.Fatalf("select event=%+v ok=%v", ev, ok)
	}

	_ = sendInteraction(t, c, "submit-1", ipc.Interaction{Type: ipc.InteractionInputSubmit, ID: "in"})
	ev, ok = d.Engine.NextEvent()
	if !ok || ev.Ev != "submit" || ev.Value != "local" {
		t.Fatalf("submit event=%+v ok=%v", ev, ok)
	}
}

func TestRemoteActionKeyExecutesDaemonRegistry(t *testing.T) {
	called := make(chan struct{}, 1)
	actions := a2runtime.NewActionRegistry(time.Second, 1)
	if err := actions.Register("reload", func(context.Context, json.RawMessage) error { called <- struct{}{}; return nil }); err != nil {
		t.Fatal(err)
	}
	d := New("session-1", protocol.DefaultLimits(), actions)
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Reload","action":"reload"}]}`)}); perr != nil {
		t.Fatal(perr)
	}
	path, _ := startTestDaemon(t, d)
	c := dialTestClient(t, path)
	defer c.conn.Close()
	_, _ = c.hello(t)
	_ = sendInteraction(t, c, "action-1", ipc.Interaction{Type: ipc.InteractionActionKey, Key: "r"})
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("action handler not called")
	}
	ev, ok := d.Engine.NextEvent()
	if !ok || ev.Ev != "action_result" || ev.Action != "reload" {
		t.Fatalf("action event=%+v ok=%v", ev, ok)
	}
}

func TestFramePublishedAckControlsDaemonPublicationExactlyOnce(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "txt", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"hello"}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpCommit, Frame: "boot"}); perr != nil {
		t.Fatal(perr)
	}
	path, _ := startTestDaemon(t, d)
	c := dialTestClient(t, path)
	defer c.conn.Close()
	_, snap := c.hello(t)
	gen := snap.Snapshot.Presentation.PublicationGeneration
	if gen == 0 || !snap.Snapshot.Presentation.PublicationPending {
		t.Fatalf("snapshot publication=%+v", snap.Snapshot.Presentation)
	}
	if _, ok := d.Engine.NextEvent(); ok {
		t.Fatal("snapshot delivery published commit before frame ack")
	}

	if err := c.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindFramePublished, PublicationGeneration: gen}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var ev protocol.Event
	var ok bool
	for time.Now().Before(deadline) {
		ev, ok = d.Engine.NextEvent()
		if ok {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ok || ev.Ev != "committed" || ev.Frame != "boot" {
		t.Fatalf("committed=%+v ok=%v", ev, ok)
	}
	if d.Engine.NeedsPublish() {
		t.Fatal("correct ACK left publication pending")
	}

	if err := c.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindFramePublished, PublicationGeneration: gen}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if ev, ok := d.Engine.NextEvent(); ok {
		t.Fatalf("duplicate ACK emitted %+v", ev)
	}
}

func TestStalePublicationAckCannotPublishNewerGeneration(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "txt", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"one"}`)}); perr != nil {
		t.Fatal(perr)
	}
	path, _ := startTestDaemon(t, d)
	c := dialTestClient(t, path)
	defer c.conn.Close()
	_, snap := c.hello(t)
	oldGen := snap.Snapshot.Presentation.PublicationGeneration

	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpProps, ID: "txt", Props: json.RawMessage(`{"text":"two"}`)}); perr != nil {
		t.Fatal(perr)
	}
	newGen, pending := d.Engine.PublicationGeneration()
	if !pending || newGen <= oldGen {
		t.Fatalf("new publication=%d pending=%v old=%d", newGen, pending, oldGen)
	}
	if err := c.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindFramePublished, PublicationGeneration: oldGen}); err != nil {
		t.Fatal(err)
	}
	msg, perr, err := c.r.Next()
	if err != nil || perr != nil {
		t.Fatalf("stale response perr=%v err=%v", perr, err)
	}
	if msg.Kind != ipc.KindError || msg.Error == nil || msg.Error.Code != "ipc.stale_publication" {
		t.Fatalf("stale response=%+v", msg)
	}
	if !d.Engine.NeedsPublish() {
		t.Fatal("stale ACK published newer state")
	}
}

func TestPendingPublicationSurvivesDisconnectAndCanBeAckedByReattachedClient(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "txt", Type: protocol.NodeText, Parent: "root", Props: json.RawMessage(`{"text":"hello"}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := d.Engine.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpCommit, Frame: "pending"}); perr != nil {
		t.Fatal(perr)
	}
	path, _ := startTestDaemon(t, d)

	c1 := dialTestClient(t, path)
	_, snap1 := c1.hello(t)
	gen := snap1.Snapshot.Presentation.PublicationGeneration
	_ = c1.conn.Close()
	deadline := time.Now().Add(time.Second)
	for d.lease.owner() != "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !d.Engine.NeedsPublish() {
		t.Fatal("disconnect faked publication")
	}

	c2 := dialTestClient(t, path)
	defer c2.conn.Close()
	_, snap2 := c2.hello(t)
	if !snap2.Snapshot.Presentation.PublicationPending || snap2.Snapshot.Presentation.PublicationGeneration != gen {
		t.Fatalf("reattach publication=%+v want generation %d", snap2.Snapshot.Presentation, gen)
	}
	if err := c2.w.Write(ipc.Message{V: ipc.Version, Kind: ipc.KindFramePublished, PublicationGeneration: gen}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for d.Engine.NeedsPublish() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if d.Engine.NeedsPublish() {
		t.Fatal("reattached current frame ACK did not publish")
	}
	ev, ok := d.Engine.NextEvent()
	if !ok || ev.Ev != "committed" || ev.Frame != "pending" {
		t.Fatalf("committed=%+v ok=%v", ev, ok)
	}
}

func TestSnapshotSignalsAreBoundedAndCoalesced(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	d.signalSnapshot()
	if got := len(d.updates); got != 1 {
		t.Fatalf("initial update depth=%d, want 1", got)
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			d.signalSnapshot()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("coalesced snapshot signaling blocked behind a slow client")
	}
	if got := len(d.updates); got != 1 {
		t.Fatalf("coalesced update depth=%d, want 1", got)
	}
}

func TestBlockingSnapshotWriteDoesNotHoldEngineMutex(t *testing.T) {
	d := New("session-1", protocol.DefaultLimits(), nil)
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	writer := ipc.NewWriter(server)

	writeDone := make(chan error, 1)
	go func() { writeDone <- d.writeSnapshot(server, writer, "") }()
	// net.Pipe writes block until the peer reads. While the snapshot write is
	// blocked, semantic Engine operations must remain independently available.
	time.Sleep(10 * time.Millisecond)
	engineDone := make(chan struct{})
	go func() {
		_ = d.Engine.PresentationSnapshot()
		close(engineDone)
	}()
	select {
	case <-engineDone:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("blocked IPC write held Engine mutex")
	}

	readDone := make(chan error, 1)
	go func() {
		_, perr, err := ipc.NewReader(client, ipc.DefaultMaxMessageBytes).Next()
		if err != nil {
			readDone <- err
			return
		}
		if perr != nil {
			readDone <- perr
			return
		}
		readDone <- nil
	}()
	if err := <-writeDone; err != nil {
		t.Fatalf("snapshot write: %v", err)
	}
	if err := <-readDone; err != nil {
		t.Fatalf("snapshot read: %v", err)
	}
}

func TestP02VisibleFrameAttributionUsesLastAckedRendererGeneration(t *testing.T) {
	d := New("p0-2-visible-attribution", protocol.DefaultLimits(), nil)
	apply := func(op protocol.Operation) {
		if perr := d.Engine.Apply(op); perr != nil {
			t.Fatal(perr)
		}
	}
	apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "targets", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"action":"pick","rows":[["a"]],"row_ids":["a"]}`)})
	apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpCommit, Frame: "A"})
	path, _ := startTestDaemon(t, d)
	c := dialTestClient(t, path)
	defer c.conn.Close()
	_, snapA := c.hello(t)
	genA := snapA.Snapshot.Presentation.PublicationGeneration
	if err := c.w.Write(ipc.Message{V: 1, Kind: ipc.KindFramePublished, PublicationGeneration: genA}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for d.Engine.NeedsPublish() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for {
		if _, ok := d.Engine.NextEvent(); !ok {
			break
		}
	}
	apply(protocol.Operation{V: 1, Seq: 3, Op: protocol.OpProps, ID: "targets", Props: json.RawMessage(`{"rows":[["b"]],"row_ids":["b"]}`)})
	apply(protocol.Operation{V: 1, Seq: 4, Op: protocol.OpCommit, Frame: "B"})
	d.signalSnapshot()
	snapB, perr, err := c.r.Next()
	if err != nil || perr != nil || snapB.Kind != ipc.KindSnapshot {
		t.Fatalf("B snapshot=%+v perr=%v err=%v", snapB, perr, err)
	}
	genB := snapB.Snapshot.Presentation.PublicationGeneration
	if genB == genA {
		t.Fatal("generation did not advance")
	}
	if err := c.w.Write(ipc.Message{V: 1, Kind: ipc.KindInteraction, Interaction: &ipc.Interaction{Type: ipc.InteractionTableActivate, ID: "targets"}}); err != nil {
		t.Fatal(err)
	}
	_, _, _ = c.r.Next()
	ev, ok := d.Engine.NextEvent()
	if !ok || ev.Ev != "select" || ev.Frame != "A" || ev.Revision != 1 {
		t.Fatalf("A-visible event=%+v ok=%v", ev, ok)
	}
	if err := c.w.Write(ipc.Message{V: 1, Kind: ipc.KindFramePublished, PublicationGeneration: genB}); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(time.Second)
	for d.Engine.NeedsPublish() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	for {
		ev, ok = d.Engine.NextEvent()
		if !ok {
			break
		}
		if ev.Ev == "committed" {
			continue
		}
		t.Fatalf("unexpected before B select: %+v", ev)
	}
	if err := c.w.Write(ipc.Message{V: 1, Kind: ipc.KindInteraction, Interaction: &ipc.Interaction{Type: ipc.InteractionTableActivate, ID: "targets"}}); err != nil {
		t.Fatal(err)
	}
	_, _, _ = c.r.Next()
	ev, ok = d.Engine.NextEvent()
	if !ok || ev.Ev != "select" || ev.Frame != "B" || ev.Revision != 2 {
		t.Fatalf("B-visible event=%+v ok=%v", ev, ok)
	}
}
