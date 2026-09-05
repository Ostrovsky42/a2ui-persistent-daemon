//go:build unix

package ipc_test

import (
    "context"
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
