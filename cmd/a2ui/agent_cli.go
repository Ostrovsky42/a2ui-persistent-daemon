package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"a2ui/ipc"
	"a2ui/protocol"
	"a2ui/transport/mcp"
	"a2ui/wire"
)

func defaultEnv(envKey, fallback string) string {
	if val := os.Getenv(envKey); val != "" {
		return val
	}
	return fallback
}

func printUsage() {
	fmt.Fprintf(os.Stdout, `a2ui - Zero-sandbox terminal UI client and agent toolkit

Usage:
  a2ui [flags]                          Launch interactive Bubble Tea TUI
  a2ui send [flags] [file.ndjson | -]   Send mutations to persistent daemon
  a2ui wait-event [flags]               Wait for user event from daemon
  a2ui status [flags]                   Query daemon status

Flags for TUI client:
  -socket string    Unix socket path (default: $XDG_RUNTIME_DIR/a2ui/a2ui.sock)
  -preset string    Presentation preset: minimal, dashboard, or dense

Flags for send:
  -server string    Daemon HTTP address (default: http://127.0.0.1:8080 or $A2UI_SERVER)
  -session string   Session ID (default: default or $A2UI_SESSION)
  -verbose          Print verbose sending progress

Flags for wait-event:
  -server string    Daemon HTTP address (default: http://127.0.0.1:8080 or $A2UI_SERVER)
  -session string   Session ID (default: default or $A2UI_SESSION)
  -timeout string   Maximum wait duration (default: 30s)
  -type string      Filter for event type (e.g. action, submit, committed)
  -raw              Output full event as JSON

Flags for status:
  -server string    Daemon HTTP address (default: http://127.0.0.1:8080 or $A2UI_SERVER)
  -json             Output raw JSON status
`)
}

func runSend(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)
	server := fs.String("server", defaultEnv("A2UI_SERVER", "http://127.0.0.1:8080"), "Daemon HTTP address")
	session := fs.String("session", defaultEnv("A2UI_SESSION", "default"), "Session ID")
	verbose := fs.Bool("verbose", false, "Print verbose sending progress")
	_ = fs.Parse(args)

	filePath := "-"
	if fs.NArg() > 0 {
		filePath = fs.Arg(0)
	}

	var r io.Reader
	if filePath == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "a2ui send: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		r = f
	}

	explicitSession := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "session" {
			explicitSession = true
		}
	})

	scanner := bufio.NewScanner(r)
	var lines [][]byte
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" && !strings.HasPrefix(text, "#") {
			lines = append(lines, []byte(text))
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "a2ui send: read input: %v\n", err)
		os.Exit(1)
	}

	// Detect session from first envelope if not explicitly overridden
	if !explicitSession && len(lines) > 0 {
		var firstEnv protocol.Envelope
		if err := json.Unmarshal(lines[0], &firstEnv); err == nil && firstEnv.Session != "" {
			*session = firstEnv.Session
		}
	}
	baseURL := strings.TrimRight(*server, "/")
	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Initial hello handshake
	helloPayload, _ := json.Marshal(protocol.Hello{
		Versions: []int{protocol.Version},
		Features: []string{"commit-barrier"},
	})
	helloEnv := protocol.Envelope{
		V:       protocol.Version,
		Session: *session,
		Kind:    protocol.KindHello,
		Seq:     0,
		Payload: helloPayload,
	}
	helloMsg, err := mcp.Request(json.RawMessage(`"hello-send"`), helloEnv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui send: build hello: %v\n", err)
		os.Exit(1)
	}
	helloBody, _ := json.Marshal(helloMsg)
	hreq, _ := http.NewRequest(http.MethodPost, baseURL+"/", bytes.NewReader(helloBody))
	for k, v := range mcp.HTTPHeaders(helloMsg) {
		hreq.Header[k] = v
	}
	hreq.Header.Set("Content-Type", "application/json")
	hresp, err := client.Do(hreq)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui send: connect to daemon %s: %v\n", baseURL, err)
		os.Exit(1)
	}
	_ = hresp.Body.Close()

	// 2. Stream operations
	var seq uint64
	count := 0

	for _, rawLine := range lines {
		var op protocol.Operation
		var opSeq uint64

		// Check if line is a full Envelope
		var env protocol.Envelope
		if err := json.Unmarshal(rawLine, &env); err == nil && env.Kind != "" {
			if env.Kind == protocol.KindHello {
				// Handshake was already performed
				continue
			}
			if env.Kind != protocol.KindOperation {
				continue
			}
			opSeq = env.Seq
			if perr := wire.StrictUnmarshal(env.Payload, &op); perr != nil {
				if err := json.Unmarshal(env.Payload, &op); err != nil {
					fmt.Fprintf(os.Stderr, "a2ui send: invalid operation payload: %v\n", err)
					os.Exit(1)
				}
			}
		} else {
			// Bare Operation: {"op":"upsert",...}
			seq++
			opSeq = seq
			if perr := wire.StrictUnmarshal(rawLine, &op); perr != nil {
				if err := json.Unmarshal(rawLine, &op); err != nil {
					fmt.Fprintf(os.Stderr, "a2ui send: invalid operation: %s\n", string(rawLine))
					os.Exit(1)
				}
			}
		}

		op.V = protocol.Version
		if op.Seq == 0 || op.Seq != int64(opSeq) {
			op.Seq = int64(opSeq)
		}

		opPayload, err := json.Marshal(op)
		if err != nil {
			fmt.Fprintf(os.Stderr, "a2ui send: encode op: %v\n", err)
			os.Exit(1)
		}
		opEnv := protocol.Envelope{
			V:       protocol.Version,
			Session: *session,
			Kind:    protocol.KindOperation,
			Seq:     opSeq,
			Payload: opPayload,
		}
		opMsg, err := mcp.Notification(opEnv)
		if err != nil {
			fmt.Fprintf(os.Stderr, "a2ui send: build op notification: %v\n", err)
			os.Exit(1)
		}
		body, _ := json.Marshal(opMsg)
		req, _ := http.NewRequest(http.MethodPost, baseURL+"/", bytes.NewReader(body))
		for k, v := range mcp.HTTPHeaders(opMsg) {
			req.Header[k] = v
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			fmt.Fprintf(os.Stderr, "a2ui send: send op seq %d: %v\n", opSeq, err)
			os.Exit(1)
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
			fmt.Fprintf(os.Stderr, "a2ui send: daemon rejected op seq %d: HTTP %d (%s)\n", opSeq, resp.StatusCode, strings.TrimSpace(string(respBody)))
			os.Exit(1)
		}
		count++
	}

	if *verbose {
		fmt.Fprintf(os.Stdout, "a2ui send: successfully sent %d operation(s) to %s (session %q)\n", count, baseURL, *session)
	}
}

func runWaitEvent(args []string) {
	fs := flag.NewFlagSet("wait-event", flag.ExitOnError)
	server := fs.String("server", defaultEnv("A2UI_SERVER", "http://127.0.0.1:8080"), "Daemon HTTP address")
	session := fs.String("session", defaultEnv("A2UI_SESSION", "default"), "Session ID")
	timeoutStr := fs.String("timeout", "30s", "Maximum wait duration")
	filterType := fs.String("type", "", "Filter for event type (e.g. action, submit, committed)")
	rawJSON := fs.Bool("raw", false, "Output full event as JSON")
	_ = fs.Parse(args)

	timeout, err := time.ParseDuration(*timeoutStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui wait-event: invalid timeout: %v\n", err)
		os.Exit(1)
	}

	baseURL := strings.TrimRight(*server, "/")
	u, _ := url.Parse(baseURL + "/events")
	q := u.Query()
	q.Set("timeout", timeout.String())
	q.Set("session", *session)
	u.RawQuery = q.Encode()

	ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui wait-event: %v\n", err)
		os.Exit(1)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui wait-event: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "a2ui wait-event: daemon error: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	var events []protocol.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		fmt.Fprintf(os.Stderr, "a2ui wait-event: decode response: %v\n", err)
		os.Exit(1)
	}

	if len(events) == 0 {
		fmt.Fprintln(os.Stderr, "a2ui wait-event: timeout exceeded with no events")
		os.Exit(2)
	}

	var matched *protocol.Event
	for i := range events {
		if *filterType == "" || events[i].Ev == *filterType {
			matched = &events[i]
			break
		}
	}

	if matched == nil {
		fmt.Fprintf(os.Stderr, "a2ui wait-event: received %d event(s) but none matched type %q\n", len(events), *filterType)
		os.Exit(2)
	}

	if *rawJSON {
		b, _ := json.Marshal(matched)
		fmt.Println(string(b))
		return
	}

	// Ergonomic key-value output for scripts
	var parts []string
	parts = append(parts, fmt.Sprintf("ev=%s", matched.Ev))
	if matched.ID != "" {
		parts = append(parts, fmt.Sprintf("id=%s", matched.ID))
	}
	if matched.Action != "" {
		parts = append(parts, fmt.Sprintf("action=%s", matched.Action))
	}
	if matched.Value != "" {
		parts = append(parts, fmt.Sprintf("value=%q", matched.Value))
	}
	if matched.RowID != "" {
		parts = append(parts, fmt.Sprintf("row_id=%s", matched.RowID))
	}
	if matched.Frame != "" {
		parts = append(parts, fmt.Sprintf("frame=%s", matched.Frame))
	}
	if matched.Code != "" {
		parts = append(parts, fmt.Sprintf("code=%s", matched.Code))
	}
	fmt.Println(strings.Join(parts, " "))
}

func runStatus(args []string) {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	server := fs.String("server", defaultEnv("A2UI_SERVER", "http://127.0.0.1:8080"), "Daemon HTTP address")
	rawJSON := fs.Bool("json", false, "Output raw JSON status")
	_ = fs.Parse(args)

	baseURL := strings.TrimRight(*server, "/")
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get(baseURL + "/status")
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui status: cannot reach daemon at %s: %v\n", baseURL, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "a2ui status: daemon error: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "a2ui status: read response: %v\n", err)
		os.Exit(1)
	}

	if *rawJSON {
		fmt.Println(string(body))
		return
	}

	var status struct {
		Session        string `json:"session"`
		Revision       uint64 `json:"revision"`
		Nodes          int    `json:"nodes"`
		HasClient      bool   `json:"has_client"`
		PendingPublish bool   `json:"pending_publish"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		fmt.Println(string(body))
		return
	}

	clientStatus := "disconnected"
	if status.HasClient {
		clientStatus = "attached"
	}

	fmt.Printf("a2uid [%s]: revision=%d nodes=%d client=%s pending_publish=%v\n",
		status.Session, status.Revision, status.Nodes, clientStatus, status.PendingPublish)
}

func runInteract(args []string) {
	fs := flag.NewFlagSet("interact", flag.ExitOnError)
	socket := fs.String("socket", "", "Unix socket path (default: $XDG_RUNTIME_DIR/a2ui/a2ui.sock)")
	submitID := fs.String("submit", "", "Submit input node by ID")
	actionKey := fs.String("action", "", "Trigger action key")
	focusID := fs.String("focus", "", "Set focus by node ID")
	inputVal := fs.String("input", "", "Set input value: id=value")
	_ = fs.Parse(args)

	path := resolveClientSocket(*socket, os.Getenv("XDG_RUNTIME_DIR"), os.Getuid())
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, perr := ipc.Dial(ctx, path, ipc.RecordLimit(protocol.DefaultLimits()))
	cancel()
	if perr != nil {
		fmt.Fprintf(os.Stderr, "a2ui interact: connect: %v\n", perr)
		os.Exit(1)
	}
	defer client.Close()

	if *inputVal != "" {
		parts := strings.SplitN(*inputVal, "=", 2)
		val := ""
		if len(parts) == 2 {
			val = parts[1]
		}
		if err := client.SetInput(parts[0], val); err != nil {
			fmt.Fprintf(os.Stderr, "a2ui interact: set-input: %v\n", err)
			os.Exit(1)
		}
	}
	if *focusID != "" {
		if err := client.Focus(*focusID); err != nil {
			fmt.Fprintf(os.Stderr, "a2ui interact: focus: %v\n", err)
			os.Exit(1)
		}
	}
	if *submitID != "" {
		if err := client.Submit(*submitID); err != nil {
			fmt.Fprintf(os.Stderr, "a2ui interact: submit: %v\n", err)
			os.Exit(1)
		}
	}
	if *actionKey != "" {
		if err := client.ActionKey(*actionKey); err != nil {
			fmt.Fprintf(os.Stderr, "a2ui interact: action: %v\n", err)
			os.Exit(1)
		}
	}
}
