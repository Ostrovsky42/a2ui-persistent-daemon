//go:build unix

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	a2tea "a2ui/adapter/bubbletea"
	a2web "a2ui/adapter/web"
	"a2ui/daemon"
	"a2ui/ipc"
	"a2ui/protocol"
	a2runtime "a2ui/runtime"
)

func dialPersistentClient(t *testing.T, path string) *ipc.Client {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		client, perr := ipc.Dial(context.Background(), path, ipc.DefaultMaxMessageBytes)
		if perr == nil {
			return client
		}
		if perr.Code != "ipc.client_busy" {
			t.Fatal(perr)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("interactive client lease was not released")
	return nil
}

func TestSameDaemonStateSurvivesTUIToWebRendererSwitch(t *testing.T) {
	actions := a2runtime.NewActionRegistry(time.Second, 1)
	approved := make(chan string, 1)
	if err := actions.Register("approve", func(_ context.Context, args json.RawMessage) error {
		var payload struct {
			Target string `json:"target"`
		}
		if err := json.Unmarshal(args, &payload); err != nil {
			return err
		}
		approved <- payload.Target
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	d := daemon.New("renderer-switch-e2e", protocol.DefaultLimits(), actions)
	path := startPersistentDaemon(t, d)
	daemonHello(t, d)
	daemonOp(t, d, 1, protocol.Operation{Op: protocol.OpUpsert, ID: "cmd", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"agent"}`)})
	daemonOp(t, d, 2, protocol.Operation{Op: protocol.OpUpsert, ID: "jobs", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"columns":[{"title":"Job"}],"selectable":true,"rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)})
	daemonOp(t, d, 3, protocol.Operation{Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"y","label":"Approve","action":"approve","args":{"target":"prod"}}]}`)})

	// Renderer A: Bubble Tea consumes the daemon snapshot and mutates only
	// semantic state through the renderer-neutral controller boundary.
	tuiClient := dialPersistentClient(t, path)
	tuiModel := a2tea.NewModelWithController(tuiClient, a2tea.DefaultTheme, a2tea.PresetMinimal, tuiClient.Updates())
	if frame := tuiModel.View(); frame == "" {
		t.Fatal("TUI rendered an empty frame")
	}
	waitPublished(t, d)
	if err := tuiClient.Focus("cmd"); err != nil {
		t.Fatal(err)
	}
	if err := tuiClient.SetInput("cmd", "survives-renderer-switch"); err != nil {
		t.Fatal(err)
	}
	if err := tuiClient.SelectTableRow("jobs", "job-b"); err != nil {
		t.Fatal(err)
	}
	if err := tuiClient.Close(); err != nil {
		t.Fatal(err)
	}

	// The agent keeps mutating the same daemon while no renderer is attached.
	daemonOp(t, d, 4, protocol.Operation{Op: protocol.OpProps, ID: "jobs", Props: json.RawMessage(`{"rows":[["B"],["A"]],"row_ids":["job-b","job-a"]}`)})
	daemonOp(t, d, 5, protocol.Operation{Op: protocol.OpCommit, Frame: "between-renderers"})
	if !d.Engine.NeedsPublish() {
		t.Fatal("daemon incorrectly considered detached changes visible")
	}

	// Renderer B: Web attaches to the same daemon through the same IPC client.
	webClient := dialPersistentClient(t, path)
	defer webClient.Close()
	server := httptest.NewServer(a2web.NewHandler(webClient))
	defer server.Close()

	resp, err := server.Client().Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	page := string(body)
	for _, want := range []string{"survives-renderer-switch", "job-b", "Approve", `method="post"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("Web renderer lost semantic state or emitted malformed controls; missing %q in %s", want, page)
		}
	}
	if strings.Contains(page, `method=\"post\"`) {
		t.Fatalf("Web renderer leaked Go string escaping into HTML: %s", page)
	}

	snapshot := webClient.Snapshot()
	if !snapshot.PublicationPending || snapshot.PublicationGeneration == 0 {
		t.Fatalf("Web attach lost pending publication: %+v", snapshot)
	}
	ackResp, err := server.Client().PostForm(server.URL+"/published", url.Values{
		"generation": {strconv.FormatUint(snapshot.PublicationGeneration, 10)},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = ackResp.Body.Close()
	if ackResp.StatusCode != http.StatusNoContent {
		t.Fatalf("publication ack status=%d", ackResp.StatusCode)
	}
	waitPublished(t, d)

	committed := false
	for _, ev := range d.DrainEvents() {
		if ev.Ev == "committed" && ev.Frame == "between-renderers" {
			committed = true
		}
	}
	if !committed {
		t.Fatal("Web visibility acknowledgement did not release the pending commit barrier")
	}

	selectResp, err := server.Client().PostForm(server.URL+"/interaction", url.Values{
		"type":   {"table_select"},
		"id":     {"jobs"},
		"row_id": {"job-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = selectResp.Body.Close()
	if selection := d.Engine.PresentationSnapshot().TableSelections["jobs"]; selection.RowID != "job-a" {
		t.Fatalf("Web semantic row selection did not reach daemon: %+v", selection)
	}

	actionResp, err := server.Client().PostForm(server.URL+"/interaction", url.Values{
		"type":   {"action_invoke"},
		"id":     {"actions"},
		"action": {"approve"},
		"args":   {`{"target":"prod"}`},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = actionResp.Body.Close()
	select {
	case target := <-approved:
		if target != "prod" {
			t.Fatalf("action target=%q", target)
		}
	case <-time.After(time.Second):
		t.Fatal("Web semantic action did not reach registered daemon capability")
	}
}
