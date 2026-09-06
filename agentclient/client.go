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

func (c *Client) Publish(ctx context.Context, ops []protocol.Operation) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.negotiated {
		if err := c.hello(ctx); err != nil {
			return err
		}
		c.negotiated = true
	}

	for _, input := range ops {
		c.nextSeq++
		seq := c.nextSeq
		op := input
		op.V = protocol.Version
		op.Seq = int64(seq)
		payload, err := json.Marshal(op)
		if err != nil {
			return fmt.Errorf("encode operation %d: %w", seq, err)
		}
		env := protocol.Envelope{
			V:       protocol.Version,
			Session: c.sessionID,
			Kind:    protocol.KindOperation,
			Seq:     seq,
			Payload: payload,
		}
		msg, err := transportmcp.Notification(env)
		if err != nil {
			return fmt.Errorf("build operation %d: %w", seq, err)
		}
		if err := c.postMCP(ctx, msg); err != nil {
			return fmt.Errorf("publish operation %d: %w", seq, err)
		}
	}
	return nil
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
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode MCP message: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	for key, values := range transportmcp.HTTPHeaders(msg) {
		req.Header[key] = append([]string(nil), values...)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("read response: %w", readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(respBody))
		var rpcErr transportmcp.RPCError
		if json.Unmarshal(respBody, &rpcErr) == nil && rpcErr.Message != "" {
			message = rpcErr.Message
		}
		return &daemonHTTPError{Status: resp.StatusCode, Message: message}
	}
	return nil
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
