//go:build unix

package e2e

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	a2tea "github.com/Ostrovsky42/agent-interaction-runtime/adapter/bubbletea"
	"github.com/Ostrovsky42/agent-interaction-runtime/daemon"
	"github.com/Ostrovsky42/agent-interaction-runtime/ipc"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func startPersistentDaemon(t *testing.T, d *daemon.Daemon) string {
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

func daemonHello(t *testing.T, d *daemon.Daemon) {
	t.Helper()
	payload, _ := json.Marshal(protocol.Hello{Versions: []int{protocol.Version}, Features: []string{"commit-barrier"}})
	if _, perr := d.HandleEnvelope(protocol.Envelope{V: protocol.Version, Session: d.Session.ID(), Kind: protocol.KindHello, Payload: payload}); perr != nil {
		t.Fatal(perr)
	}
}

func daemonOp(t *testing.T, d *daemon.Daemon, seq uint64, op protocol.Operation) {
	t.Helper()
	op.V, op.Seq = protocol.Version, int64(seq)
	payload, _ := json.Marshal(op)
	if _, perr := d.HandleEnvelope(protocol.Envelope{V: protocol.Version, Session: d.Session.ID(), Kind: protocol.KindOperation, Seq: seq, Payload: payload}); perr != nil {
		t.Fatal(perr)
	}
}

func waitPublished(t *testing.T, d *daemon.Daemon) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for d.Engine.NeedsPublish() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if d.Engine.NeedsPublish() {
		t.Fatal("publication ACK did not reach daemon")
	}
}

func TestPersistentDaemonReattachPreservesSemanticStateAndRemotePublicationBarrier(t *testing.T) {
	d := daemon.New("persistent-e2e", protocol.DefaultLimits(), nil)
	path := startPersistentDaemon(t, d)
	daemonHello(t, d)
	daemonOp(t, d, 1, protocol.Operation{Op: protocol.OpUpsert, ID: "cmd", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent"}`)})
	daemonOp(t, d, 2, protocol.Operation{Op: protocol.OpUpsert, ID: "jobs", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)})

	c1, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
	if perr != nil {
		t.Fatal(perr)
	}
	m1 := a2tea.NewModelWithController(c1, a2tea.DefaultTheme, a2tea.PresetMinimal, c1.Updates())
	_ = m1.View()
	waitPublished(t, d)
	if err := c1.Focus("cmd"); err != nil {
		t.Fatal(err)
	}
	if err := c1.SetInput("cmd", "persistent-value"); err != nil {
		t.Fatal(err)
	}
	if err := c1.MoveTableSelection("jobs", 1); err != nil {
		t.Fatal(err)
	}
	if got := d.Engine.PresentationSnapshot().TableSelections["jobs"].RowID; got != "job-b" {
		t.Fatalf("selection=%q", got)
	}
	if err := c1.Close(); err != nil {
		t.Fatal(err)
	}

	// Agent keeps working with no terminal attached. Reorder the table while
	// preserving stable row identity, then create a publication barrier.
	daemonOp(t, d, 3, protocol.Operation{Op: protocol.OpProps, ID: "jobs", Props: json.RawMessage(`{"rows":[["B"],["A"]],"row_ids":["job-b","job-a"]}`)})
	daemonOp(t, d, 4, protocol.Operation{Op: protocol.OpCommit, Frame: "while-detached"})
	if !d.Engine.NeedsPublish() {
		t.Fatal("daemon faked publication while no client was attached")
	}

	var c2 *ipc.Client
	deadline := time.Now().Add(time.Second)
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
		t.Fatal("client B could not reattach")
	}
	defer c2.Close()
	snap := c2.Snapshot()
	if snap.FocusedID != "cmd" || snap.InputValues["cmd"] != "persistent-value" {
		t.Fatalf("reattach state=%+v", snap)
	}
	selection := snap.TableSelections["jobs"]
	if selection.RowID != "job-b" || selection.Index != 0 {
		t.Fatalf("selection after reorder=%+v", selection)
	}
	if !snap.PublicationPending {
		t.Fatal("reattached snapshot lost pending publication")
	}

	m2 := a2tea.NewModelWithController(c2, a2tea.DefaultTheme, a2tea.PresetDashboard, c2.Updates())
	if frame := m2.View(); frame == "" {
		t.Fatal("reattached renderer produced empty frame")
	}
	waitPublished(t, d)
	events := d.DrainEvents()
	found := false
	for _, ev := range events {
		if ev.Ev == "committed" && ev.Frame == "while-detached" {
			found = true
		}
	}
	if !found {
		t.Fatalf("committed event missing after visible reattach frame: %+v", events)
	}
}
