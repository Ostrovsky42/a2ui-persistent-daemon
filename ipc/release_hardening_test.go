//go:build unix

package ipc_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"a2ui/daemon"
	"a2ui/ipc"
	"a2ui/protocol"
)

func TestClientCloseReleasesInteractiveLeaseBeforeReturning(t *testing.T) {
	d := daemon.New("session", protocol.DefaultLimits(), nil)
	path := startDaemonForClientTest(t, d)

	for i := 0; i < 100; i++ {
		c1, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
		if perr != nil {
			t.Fatalf("dial client A iteration %d: %v", i, perr)
		}
		if err := c1.Close(); err != nil {
			t.Fatalf("close client A iteration %d: %v", i, err)
		}

		c2, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
		if perr != nil {
			t.Fatalf("immediate reattach iteration %d: %v", i, perr)
		}
		if err := c2.Close(); err != nil {
			t.Fatalf("close client B iteration %d: %v", i, err)
		}
	}
}

func TestIPCRecordLimitCoversRetainedDocumentBudget(t *testing.T) {
	limits := protocol.DefaultLimits()
	got := ipc.RecordLimit(limits)
	if got <= limits.MaxDocumentBytes {
		t.Fatalf("RecordLimit=%d must exceed MaxDocumentBytes=%d to allow snapshot JSON expansion", got, limits.MaxDocumentBytes)
	}
	if got < limits.MaxMessageBytes {
		t.Fatalf("RecordLimit=%d must not be below agent record limit=%d", got, limits.MaxMessageBytes)
	}
}

func TestClientCanAttachToSnapshotLargerThanAgentRecordLimit(t *testing.T) {
	limits := protocol.DefaultLimits()
	d := daemon.New("session", limits, nil)
	path := startDaemonForClientTest(t, d)
	negotiateAgent(t, d)

	chunk := strings.Repeat("x", 220<<10)
	seq := uint64(1)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("large-%d", i)
		agentOp(t, d, seq, protocol.Operation{Op: protocol.OpUpsert, ID: id, Type: protocol.NodeText, Parent: "root", Props: []byte(`{}`)})
		seq++
		agentOp(t, d, seq, protocol.Operation{Op: protocol.OpText, ID: id, Text: chunk})
		seq++
	}
	if got := d.Engine.PresentationSnapshot().Document.TotalTextBytes; got <= limits.MaxMessageBytes {
		t.Fatalf("test document must exceed agent record limit: text=%d agent_limit=%d", got, limits.MaxMessageBytes)
	}

	client, perr := ipc.Dial(context.Background(), path, ipc.RecordLimit(limits))
	if perr != nil {
		t.Fatalf("attach with large retained document: %v", perr)
	}
	defer client.Close()
	if got := client.Snapshot().Document.TotalTextBytes; got <= limits.MaxMessageBytes {
		t.Fatalf("large snapshot was not delivered: text=%d agent_limit=%d", got, limits.MaxMessageBytes)
	}
}
