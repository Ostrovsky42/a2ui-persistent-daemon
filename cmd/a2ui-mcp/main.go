package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"a2ui/agentclient"
	"a2ui/protocol"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type operationInput struct {
	Op     string         `json:"op" jsonschema:"A2UI V1 mutation: upsert, props, text, remove, focus, or commit"`
	ID     string         `json:"id,omitempty" jsonschema:"target node ID"`
	Type   string         `json:"type,omitempty" jsonschema:"node type for upsert"`
	Parent string         `json:"parent,omitempty" jsonschema:"parent node ID for upsert"`
	Index  *int           `json:"index,omitempty" jsonschema:"optional child index for upsert"`
	Text   string         `json:"text,omitempty" jsonschema:"text payload for text mutation"`
	Frame  string         `json:"frame,omitempty" jsonschema:"frame label for commit"`
	Props  map[string]any `json:"props,omitempty" jsonschema:"node property object for upsert or props"`
}

type publishInput struct {
	Operations []operationInput `json:"operations" jsonschema:"ordered A2UI V1 operations to publish"`
}

type publishOutput struct {
	Published int `json:"published"`
}

type waitEventInput struct {
	TimeoutMS  int      `json:"timeout_ms,omitempty" jsonschema:"overall wait timeout in milliseconds; default 30000, maximum 120000"`
	EventTypes []string `json:"event_types,omitempty" jsonschema:"optional event type allowlist such as submit or select"`
}

type waitEventOutput struct {
	MatchedEvents  []protocol.Event `json:"matched_events"`
	ObservedEvents []protocol.Event `json:"observed_events"`
	TimedOut       bool             `json:"timed_out"`
}

type statusInput struct{}

func newMCPServer(client *agentclient.Client) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "a2ui-mcp", Version: "p0"}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "a2ui_publish",
		Description: "Publish ordered A2UI V1 operations through the persistent daemon's existing Session and Engine authority.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input publishInput) (*mcp.CallToolResult, publishOutput, error) {
		if client == nil {
			return nil, publishOutput{}, errors.New("A2UI daemon client is unavailable")
		}
		if len(input.Operations) == 0 {
			return nil, publishOutput{}, errors.New("operations must not be empty")
		}
		ops := make([]protocol.Operation, 0, len(input.Operations))
		for i, in := range input.Operations {
			op, err := toOperation(in)
			if err != nil {
				return nil, publishOutput{}, fmt.Errorf("operation %d: %w", i, err)
			}
			ops = append(ops, op)
		}
		if err := client.Publish(ctx, ops); err != nil {
			return nil, publishOutput{}, err
		}
		return nil, publishOutput{Published: len(ops)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "a2ui_wait_event",
		Description: "Wait for semantic A2UI events. Optional filtering keeps non-matching events visible in observed_events.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input waitEventInput) (*mcp.CallToolResult, waitEventOutput, error) {
		if client == nil {
			return nil, waitEventOutput{}, errors.New("A2UI daemon client is unavailable")
		}
		return waitForEvent(ctx, client, input)
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "a2ui_status",
		Description: "Read persistent A2UI daemon status, including whether a real interactive terminal client is attached.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ statusInput) (*mcp.CallToolResult, agentclient.Status, error) {
		if client == nil {
			return nil, agentclient.Status{}, errors.New("A2UI daemon client is unavailable")
		}
		status, err := client.Status(ctx)
		return nil, status, err
	})

	return server
}

func toOperation(in operationInput) (protocol.Operation, error) {
	op := protocol.Operation{
		Op:     protocol.OpType(in.Op),
		ID:     in.ID,
		Type:   protocol.NodeType(in.Type),
		Parent: in.Parent,
		Index:  in.Index,
		Text:   in.Text,
		Frame:  in.Frame,
	}
	if in.Props != nil {
		props, err := json.Marshal(in.Props)
		if err != nil {
			return protocol.Operation{}, fmt.Errorf("encode props: %w", err)
		}
		op.Props = props
	}
	return op, nil
}

func waitForEvent(ctx context.Context, client *agentclient.Client, input waitEventInput) (*mcp.CallToolResult, waitEventOutput, error) {
	timeoutMS := input.TimeoutMS
	if timeoutMS == 0 {
		timeoutMS = 30000
	}
	if timeoutMS < 0 || timeoutMS > 120000 {
		return nil, waitEventOutput{}, fmt.Errorf("timeout_ms must be between 0 and 120000")
	}

	wanted := make(map[string]struct{}, len(input.EventTypes))
	for _, eventType := range input.EventTypes {
		if eventType == "" {
			return nil, waitEventOutput{}, errors.New("event_types must not contain empty values")
		}
		wanted[eventType] = struct{}{}
	}

	deadline := time.Now().Add(time.Duration(timeoutMS) * time.Millisecond)
	out := waitEventOutput{
		MatchedEvents:  []protocol.Event{},
		ObservedEvents: []protocol.Event{},
	}

	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			out.TimedOut = true
			return nil, out, nil
		}
		events, err := client.WaitEvents(ctx, remaining)
		if err != nil {
			return nil, waitEventOutput{}, err
		}
		if len(events) == 0 {
			out.TimedOut = true
			return nil, out, nil
		}
		out.ObservedEvents = append(out.ObservedEvents, events...)
		if len(wanted) == 0 {
			out.MatchedEvents = append(out.MatchedEvents, events...)
			return nil, out, nil
		}
		for _, event := range events {
			if _, ok := wanted[event.Ev]; ok {
				out.MatchedEvents = append(out.MatchedEvents, event)
			}
		}
		if len(out.MatchedEvents) > 0 {
			return nil, out, nil
		}
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := agentclient.New(
		envOrDefault("A2UI_SERVER", "http://127.0.0.1:8080"),
		envOrDefault("A2UI_SESSION", "default"),
		nil,
	)
	if err := newMCPServer(client).Run(ctx, &mcp.StdioTransport{}); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "a2ui-mcp: %v\n", err)
		os.Exit(1)
	}
}
