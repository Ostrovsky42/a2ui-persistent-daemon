package main

import (
	"bytes"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"a2ui/daemon"
	"a2ui/protocol"
)

func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()
	_ = w.Close()
	os.Stdout = old
	return <-outC
}

func TestAgentCLIStatusAndSendAndEvents(t *testing.T) {
	d := daemon.New("agent-test", protocol.DefaultLimits(), nil)
	ts := httptest.NewServer(d)
	defer ts.Close()

	// 1. Test status subcommand
	outStatus := captureStdout(func() {
		runStatus([]string{"-server", ts.URL, "-json"})
	})
	if !strings.Contains(outStatus, `"session":"agent-test"`) {
		t.Fatalf("status output unexpected: %s", outStatus)
	}

	// 2. Test send subcommand with stdin input
	fixture := `{"op":"upsert","id":"cli-title","type":"text","parent":"root","props":{"text":"CLI working!"}}`
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		_, _ = w.WriteString(fixture + "\n")
		_ = w.Close()
	}()
	runSend([]string{"-server", ts.URL, "-session", "agent-test", "-"})
	os.Stdin = oldStdin

	if _, ok := d.Engine.Document().Nodes["cli-title"]; !ok {
		t.Fatal("send subcommand failed to upsert cli-title into daemon")
	}

	// 3. Simulate user interaction and test wait-event subcommand
	_ = d.Engine.Apply(protocol.Operation{
		V:      1,
		Seq:    2,
		Op:     protocol.OpUpsert,
		ID:     "cli-input",
		Type:   protocol.NodeInput,
		Parent: "root",
	})
	_ = d.Engine.SetInput("cli-input", "hello from user")
	_ = d.Engine.Submit("cli-input")

	outEvent := captureStdout(func() {
		runWaitEvent([]string{"-server", ts.URL, "-session", "agent-test", "-timeout", "2s"})
	})

	if !strings.Contains(outEvent, "ev=submit") || !strings.Contains(outEvent, "id=cli-input") || !strings.Contains(outEvent, `value="hello from user"`) {
		t.Fatalf("wait-event output unexpected: %s", outEvent)
	}
}
