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

type publishOutput = agentclient.PublishReceipt

type waitEventInput struct {
	TimeoutMS  int      `json:"timeout_ms,omitempty" jsonschema:"overall wait timeout in milliseconds; default 30000, maximum 120000"`
	AfterSeq   uint64   `json:"after_seq,omitempty" jsonschema:"enqueue cursor boundary from a2ui_publish receipt"`
	Frame      string   `json:"frame,omitempty"`
	Revision   uint64   `json:"revision,omitempty"`
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
		receipt, err := client.PublishReceipt(ctx, ops)
		if err != nil {
			return nil, publishOutput{}, err
		}
		return nil, receipt, nil
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
	if input.AfterSeq == 0 && input.Frame == "" && input.Revision == 0 {
		observed := []protocol.Event{}
		for {
			events, err := client.WaitEvents(ctx, time.Until(deadline))
			if err != nil {
				return nil, waitEventOutput{}, err
			}
			observed = append(observed, events...)
			matched := []protocol.Event{}
			for _, ev := range events {
				if len(input.EventTypes) == 0 {
					matched = append(matched, ev)
					continue
				}
				for _, typ := range input.EventTypes {
					if ev.Ev == typ {
						matched = append(matched, ev)
						break
					}
				}
			}
			if len(matched) > 0 || len(events) == 0 {
				return nil, waitEventOutput{MatchedEvents: matched, ObservedEvents: observed, TimedOut: len(matched) == 0}, nil
			}
		}
	}

	result, err := client.WaitSemanticEvents(ctx, agentclient.WaitRequest{AfterSeq: input.AfterSeq, Frame: input.Frame, Revision: input.Revision, EventTypes: input.EventTypes, TimeoutMS: int(time.Until(deadline).Milliseconds())})
	if err != nil {
		// Compatibility fallback for older daemon bridges; current daemon uses causal waiter above.
		events, legacyErr := client.WaitEvents(ctx, time.Until(deadline))
		if legacyErr != nil {
			return nil, waitEventOutput{}, err
		}
		matched := make([]protocol.Event, 0)
		for _, ev := range events {
			if len(input.EventTypes) == 0 {
				matched = append(matched, ev)
				continue
			}
			for _, typ := range input.EventTypes {
				if ev.Ev == typ {
					matched = append(matched, ev)
					break
				}
			}
		}
		return nil, waitEventOutput{MatchedEvents: matched, ObservedEvents: events, TimedOut: len(matched) == 0}, nil
	}
	return nil, waitEventOutput{MatchedEvents: result.MatchedEvents, ObservedEvents: result.ObservedEvents, TimedOut: result.TimedOut}, nil
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
