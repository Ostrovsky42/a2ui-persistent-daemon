package e2e

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/layout"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	"github.com/Ostrovsky42/agent-interaction-runtime/transport/mcp"
	"github.com/Ostrovsky42/agent-interaction-runtime/wire"
)

// AgentSimulator models an autonomous AI agent driving an A2UI session via
// the MCP 2026-07-28 protocol specification.
type AgentSimulator struct {
	SessionID string
	Runner    *Runner
	seq       uint64
	Ack       *protocol.HelloAck
	events    []protocol.Event
}

// NewAgentSimulator creates an agent connected to a Runner.
func NewAgentSimulator(sessionID string, runner *Runner) *AgentSimulator {
	return &AgentSimulator{
		SessionID: sessionID,
		Runner:    runner,
	}
}

// Connect performs the a2ui/hello handshake over MCP.
func (a *AgentSimulator) Connect(features ...string) error {
	if len(features) == 0 {
		features = []string{"commit-barrier", "incremental-props", "action-events", "bounded-resources"}
	}
	hello := protocol.Hello{
		Versions: []int{protocol.Version},
		Features: features,
	}
	payload, err := json.Marshal(hello)
	if err != nil {
		return err
	}
	env := protocol.Envelope{
		V:       protocol.Version,
		Session: a.SessionID,
		Kind:    protocol.KindHello,
		Seq:     0,
		Payload: payload,
	}
	msg, err := mcp.Request(json.RawMessage(`"hello-1"`), env)
	if err != nil {
		return err
	}

	resp, perr := a.Runner.HandleMCPMessage(msg)
	if perr != nil {
		return fmt.Errorf("hello failed: %s", perr.Message)
	}
	if resp == nil {
		return fmt.Errorf("expected hello_ack response, got nil")
	}

	ackEnv, perr := mcp.EnvelopeFromMessage(*resp)
	if perr != nil {
		return fmt.Errorf("invalid hello_ack envelope: %s", perr.Message)
	}
	if ackEnv.Kind != protocol.KindHelloAck {
		return fmt.Errorf("expected hello_ack, got %s", ackEnv.Kind)
	}

	var ack protocol.HelloAck
	if perr := wire.StrictUnmarshal(ackEnv.Payload, &ack); perr != nil {
		return fmt.Errorf("unmarshal hello_ack: %s", perr.Message)
	}
	a.Ack = &ack
	return nil
}

// SendOp increments the mutation sequence and dispatches an Operation over MCP.
func (a *AgentSimulator) SendOp(op protocol.Operation) error {
	a.seq++
	op.V = protocol.Version
	op.Seq = int64(a.seq)

	payload, err := json.Marshal(op)
	if err != nil {
		return err
	}

	env := protocol.Envelope{
		V:       protocol.Version,
		Session: a.SessionID,
		Kind:    protocol.KindOperation,
		Seq:     a.seq,
		Payload: payload,
	}

	msg, err := mcp.Notification(env)
	if err != nil {
		return err
	}

	_, perr := a.Runner.HandleMCPMessage(msg)
	if perr != nil {
		return fmt.Errorf("op %s failed: [%s] %s", op.Op, perr.Code, perr.Message)
	}
	return nil
}

// Upsert creates or replaces a node.
func (a *AgentSimulator) Upsert(id, parent string, nodeType protocol.NodeType, props map[string]any, index *int) error {
	var propsRaw json.RawMessage
	if props != nil {
		b, err := json.Marshal(props)
		if err != nil {
			return err
		}
		propsRaw = b
	} else {
		propsRaw = json.RawMessage(`{}`)
	}

	return a.SendOp(protocol.Operation{
		Op:     protocol.OpUpsert,
		ID:     id,
		Type:   nodeType,
		Parent: parent,
		Index:  index,
		Props:  propsRaw,
	})
}

// UpdateProps updates props on an existing node.
func (a *AgentSimulator) UpdateProps(id string, props map[string]any) error {
	b, err := json.Marshal(props)
	if err != nil {
		return err
	}
	return a.SendOp(protocol.Operation{
		Op:    protocol.OpProps,
		ID:    id,
		Props: b,
	})
}

// AppendText appends streaming text chunk to a node.
func (a *AgentSimulator) AppendText(id, chunk string) error {
	return a.SendOp(protocol.Operation{
		Op:   protocol.OpText,
		ID:   id,
		Text: chunk,
	})
}

// Remove deletes a node and its subtree.
func (a *AgentSimulator) Remove(id string) error {
	return a.SendOp(protocol.Operation{
		Op: protocol.OpRemove,
		ID: id,
	})
}

// Focus requests focus on an interactive node.
func (a *AgentSimulator) Focus(id string) error {
	return a.SendOp(protocol.Operation{
		Op: protocol.OpFocus,
		ID: id,
	})
}

// Commit publishes authoritative state through the commit barrier.
func (a *AgentSimulator) Commit(frame string) error {
	return a.SendOp(protocol.Operation{
		Op:    protocol.OpCommit,
		Frame: frame,
	})
}

// FetchEvents drains notifications from the runner into the agent's event log.
func (a *AgentSimulator) FetchEvents() ([]protocol.Event, error) {
	msgs, err := a.Runner.DrainMCPNotifications()
	if err != nil {
		return nil, err
	}
	var newEvents []protocol.Event
	for _, m := range msgs {
		env, perr := mcp.EnvelopeFromMessage(m)
		if perr != nil {
			return nil, fmt.Errorf("invalid event envelope: %s", perr.Message)
		}
		var ev protocol.Event
		if err := json.Unmarshal(env.Payload, &ev); err != nil {
			return nil, err
		}
		newEvents = append(newEvents, ev)
		a.events = append(a.events, ev)
	}
	return newEvents, nil
}

// WaitEvent waits for an event matching evType.
func (a *AgentSimulator) WaitEvent(evType string, timeout time.Duration) (*protocol.Event, error) {
	start := time.Now()
	for time.Since(start) < timeout {
		events, err := a.FetchEvents()
		if err != nil {
			return nil, err
		}
		for _, ev := range events {
			if ev.Ev == evType {
				return &ev, nil
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for event %q", evType)
}

// SetupFormScreen sets up a complete user interaction form.
func (a *AgentSimulator) SetupFormScreen() error {
	// Container box
	if err := a.Upsert("card", "root", protocol.NodeBox, map[string]any{
		"dir":     string(layout.Column),
		"border":  string(layout.BorderRounded),
		"padding": 1,
		"gap":     1,
		"style": map[string]any{
			"fg":   "primary",
			"bold": true,
		},
	}, nil); err != nil {
		return err
	}

	// Title
	if err := a.Upsert("title", "card", protocol.NodeText, map[string]any{
		"text": "A2UI Agent Service Setup",
		"style": map[string]any{
			"fg":   "primary",
			"bold": true,
		},
	}, nil); err != nil {
		return err
	}

	// Prompt label
	if err := a.Upsert("label", "card", protocol.NodeText, map[string]any{
		"text": "Please provide operator identity:",
		"style": map[string]any{
			"dim": true,
		},
	}, nil); err != nil {
		return err
	}

	// Input
	if err := a.Upsert("username", "card", protocol.NodeInput, map[string]any{
		"placeholder": "Enter username...",
		"value":       "",
		"action":      "submit_login",
	}, nil); err != nil {
		return err
	}

	// Actions
	if err := a.Upsert("actions", "card", protocol.NodeActions, map[string]any{
		"items": []map[string]any{
			{"key": "s", "label": "Submit", "action": "submit_login"},
			{"key": "q", "label": "Quit", "action": "quit"},
		},
	}, nil); err != nil {
		return err
	}

	// Focus input
	if err := a.Focus("username"); err != nil {
		return err
	}

	// Commit initial view
	return a.Commit("form-initial")
}

// CompleteFormSuccess replaces the form with a success confirmation card.
func (a *AgentSimulator) CompleteFormSuccess(username string) error {
	// Remove actions and inputs
	_ = a.Remove("actions")
	_ = a.Remove("username")
	_ = a.Remove("label")

	// Update title
	_ = a.UpdateProps("title", map[string]any{
		"text": fmt.Sprintf("Welcome, %s! (Session Authenticated)", username),
		"style": map[string]any{
			"fg":   "primary",
			"bold": true,
		},
	})

	// Add progress bar 100%
	if err := a.Upsert("pbar", "card", protocol.NodeProgress, map[string]any{
		"value": 1.0,
		"label": "Ready",
	}, nil); err != nil {
		return err
	}

	// Add status text
	if err := a.Upsert("status", "card", protocol.NodeText, map[string]any{
		"text": "All system parameters verified and committed.",
		"style": map[string]any{
			"fg": "warn",
		},
	}, nil); err != nil {
		return err
	}

	return a.Commit("form-success")
}
