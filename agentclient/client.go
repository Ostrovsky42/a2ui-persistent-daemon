package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"a2ui/protocol"
	transportmcp "a2ui/transport/mcp"
)

var ErrAgentStreamConflict = errors.New("agent stream conflict")

type PublishReceipt struct {
	Published             int    `json:"published"`
	Frame                 string `json:"frame"`
	Revision              uint64 `json:"revision"`
	PublicationGeneration uint64 `json:"publication_generation"`
	EventCursor           uint64 `json:"event_cursor"`
	Visible               bool   `json:"visible"`
}

type WaitRequest struct {
	AfterSeq   uint64   `json:"after_seq"`
	Frame      string   `json:"frame,omitempty"`
	Revision   uint64   `json:"revision,omitempty"`
	EventTypes []string `json:"event_types,omitempty"`
	TimeoutMS  int      `json:"timeout_ms,omitempty"`
}
type WaitResult struct {
	MatchedEvents  []protocol.Event `json:"matched_events"`
	ObservedEvents []protocol.Event `json:"observed_events"`
	TimedOut       bool             `json:"timed_out"`
}

func (c *Client) WaitSemanticEvents(ctx context.Context, in WaitRequest) (WaitResult, error) {
	params, err := json.Marshal(in)
	if err != nil {
		return WaitResult{}, err
	}
	msg := transportmcp.Message{JSONRPC: "2.0", ID: json.RawMessage(`"wait-agentclient"`), Method: "a2ui/wait_event", Params: params}
	body, err := c.postMCPBody(ctx, msg)
	if err != nil {
		return WaitResult{}, err
	}
	var out transportmcp.Message
	if err := json.Unmarshal(body, &out); err != nil {
		return WaitResult{}, err
	}
	var result WaitResult
	if err := json.Unmarshal(out.Result, &result); err != nil {
		return WaitResult{}, fmt.Errorf("decode wait result: %w", err)
	}
	return result, nil
}

type Status struct {
	Session        string `json:"session"`
	Revision       uint64 `json:"revision"`
	Nodes          int    `json:"nodes"`
	HasClient      bool   `json:"has_client"`
	Generation     uint64 `json:"generation"`
	PendingPublish bool   `json:"pending_publish"`
}

type daemonHTTPError struct {
	Status  int
	Message string
}

func (e *daemonHTTPError) Error() string {
	return fmt.Sprintf("daemon HTTP %d: %s", e.Status, e.Message)
}

type Client struct {
	baseURL    string
	sessionID  string
	httpClient *http.Client

	mu         sync.Mutex
	negotiated bool
	nextSeq    uint64
}

func New(serverURL, sessionID string, httpClient *http.Client) *Client {
	serverURL = strings.TrimRight(serverURL, "/")
	if serverURL == "" {
		serverURL = "http://127.0.0.1:8080"
	}
	if sessionID == "" {
		sessionID = "default"
	}
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &Client{baseURL: serverURL, sessionID: sessionID, httpClient: httpClient}
}

func (c *Client) PublishReceipt(ctx context.Context, ops []protocol.Operation) (PublishReceipt, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.negotiated {
		if err := c.hello(ctx); err != nil {
			return PublishReceipt{}, err
		}
		c.negotiated = true
	}
	if len(ops) == 0 {
		return PublishReceipt{}, nil
	}
	batch := struct {
		Session    string               `json:"session"`
		Operations []protocol.Operation `json:"operations"`
	}{Session: c.sessionID, Operations: make([]protocol.Operation, 0, len(ops))}
	startSeq := c.nextSeq
	for i, input := range ops {
		op := input
		op.V = protocol.Version
		op.Seq = int64(startSeq + uint64(i) + 1)
		batch.Operations = append(batch.Operations, op)
	}
	params, err := json.Marshal(batch)
	if err != nil {
		return PublishReceipt{}, fmt.Errorf("encode publish batch: %w", err)
	}
	msg := transportmcp.Message{JSONRPC: "2.0", ID: json.RawMessage(`"publish-agentclient"`), Method: "a2ui/publish_batch", Params: params}
	body, err := c.postMCPBody(ctx, msg)
	if err != nil {
		return PublishReceipt{}, fmt.Errorf("publish batch: %w", err)
	}
	var out transportmcp.Message
	if err := json.Unmarshal(body, &out); err != nil {
		return PublishReceipt{}, fmt.Errorf("decode publish response: %w", err)
	}
	var receipt PublishReceipt
	if err := json.Unmarshal(out.Result, &receipt); err != nil {
		return PublishReceipt{}, fmt.Errorf("decode publish receipt: %w", err)
	}
	c.nextSeq += uint64(len(ops))
	return receipt, nil
}

func (c *Client) Publish(ctx context.Context, ops []protocol.Operation) error {
	_, err := c.PublishReceipt(ctx, ops)
	return err
}

func (c *Client) hello(ctx context.Context) error {
	payload, err := json.Marshal(protocol.Hello{
		Versions: []int{protocol.Version},
		Features: []string{"commit-barrier"},
	})
	if err != nil {
		return fmt.Errorf("encode hello: %w", err)
	}
	env := protocol.Envelope{
		V:       protocol.Version,
		Session: c.sessionID,
		Kind:    protocol.KindHello,
		Payload: payload,
	}
	msg, err := transportmcp.Request(json.RawMessage(`"hello-agentclient"`), env)
	if err != nil {
		return fmt.Errorf("build hello: %w", err)
	}
	if err := c.postMCP(ctx, msg); err != nil {
		var daemonErr *daemonHTTPError
		if errors.As(err, &daemonErr) && daemonErr.Status == http.StatusUnprocessableEntity && strings.Contains(strings.ToLower(daemonErr.Message), "hello only allowed in new") {
			return fmt.Errorf("%w: daemon session %q is already negotiated by another/previous agent stream; start a fresh daemon/session before reconnecting: %v", ErrAgentStreamConflict, c.sessionID, err)
		}
		return fmt.Errorf("hello: %w", err)
	}
	return nil
}

func (c *Client) postMCP(ctx context.Context, msg transportmcp.Message) error {
	_, err := c.postMCPBody(ctx, msg)
	return err
}

func (c *Client) postMCPBody(ctx context.Context, msg transportmcp.Message) ([]byte, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode MCP message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for key, values := range transportmcp.HTTPHeaders(msg) {
		req.Header[key] = append([]string(nil), values...)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, fmt.Errorf("read response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(respBody))
		var rpcErr transportmcp.RPCError
		if json.Unmarshal(respBody, &rpcErr) == nil && rpcErr.Message != "" {
			message = rpcErr.Message
		}
		return nil, &daemonHTTPError{Status: resp.StatusCode, Message: message}
	}
	return respBody, nil
}

func (c *Client) WaitEvents(ctx context.Context, timeout time.Duration) ([]protocol.Event, error) {
	endpoint := c.baseURL + "/events?timeout=" + url.QueryEscape(timeout.String())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build events request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("events request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var events []protocol.Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("decode events: %w", err)
	}
	return events, nil
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/status", nil)
	if err != nil {
		return Status{}, fmt.Errorf("build status request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("status request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return Status{}, fmt.Errorf("daemon HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var status Status
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return Status{}, fmt.Errorf("decode status: %w", err)
	}
	return status, nil
}
