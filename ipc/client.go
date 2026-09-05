//go:build unix

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"a2ui/engine"
	a2runtime "a2ui/runtime"
)

const defaultRequestTimeout = 3 * time.Second

type clientResponse struct {
	err *Error
}

// Client is a thin semantic controller over the daemon's Unix socket. Its
// snapshot is only a local immutable projection cache; the daemon remains the
// authority for Document and Runtime state.
type Client struct {
	conn   net.Conn
	reader *Reader
	writer *Writer
	max    int

	mu            sync.RWMutex
	snapshot      engine.PresentationSnapshot
	clientID      string
	lastErr       error
	connectionErr error

	updates chan struct{}
	closed  chan struct{}
	once    sync.Once

	pendingMu sync.Mutex
	pending   map[string]chan clientResponse
	nextReq   atomic.Uint64
}

func Dial(ctx context.Context, path string, maxMessageBytes int) (*Client, *Error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxMessageBytes < 1 {
		maxMessageBytes = DefaultMaxMessageBytes
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, NewError("ipc.connection_closed", err.Error())
	}
	c := &Client{
		conn:    conn,
		reader:  NewReader(conn, maxMessageBytes),
		writer:  NewWriter(conn),
		max:     maxMessageBytes,
		updates: make(chan struct{}, 1),
		closed:  make(chan struct{}),
		pending: make(map[string]chan clientResponse),
	}
	fail := func(perr *Error) (*Client, *Error) {
		_ = conn.Close()
		return nil, perr
	}
	if err := c.writer.Write(Message{V: Version, Kind: KindHello, Client: "bubbletea"}); err != nil {
		return fail(NewError("ipc.connection_closed", err.Error()))
	}
	ack, perr, err := c.reader.Next()
	if err != nil {
		return fail(NewError("ipc.connection_closed", err.Error()))
	}
	if perr != nil {
		return fail(perr)
	}
	if ack.Kind == KindError && ack.Error != nil {
		return fail(ack.Error)
	}
	if ack.Kind != KindHelloAck || ack.ClientID == "" || !ack.Interactive {
		return fail(NewError("ipc.invalid_message", "daemon did not return an interactive hello_ack"))
	}
	initial, perr, err := c.reader.Next()
	if err != nil {
		return fail(NewError("ipc.connection_closed", err.Error()))
	}
	if perr != nil {
		return fail(perr)
	}
	if initial.Kind != KindSnapshot || initial.Snapshot == nil {
		return fail(NewError("ipc.invalid_message", "daemon did not send initial snapshot"))
	}
	c.clientID = ack.ClientID
	c.snapshot = clonePresentation(initial.Snapshot.Presentation)
	go c.readLoop()
	return c, nil
}

func clonePresentation(in engine.PresentationSnapshot) engine.PresentationSnapshot {
	out := engine.PresentationSnapshot{
		Document:              in.Document.Clone(),
		FocusedID:             in.FocusedID,
		InputValues:           make(map[string]string, len(in.InputValues)),
		TableSelections:       make(map[string]a2runtime.TableSelection, len(in.TableSelections)),
		Bindings:              make(map[string]a2runtime.Binding, len(in.Bindings)),
		PublicationGeneration: in.PublicationGeneration,
		PublicationPending:    in.PublicationPending,
	}
	for id, value := range in.InputValues {
		out.InputValues[id] = value
	}
	for id, selection := range in.TableSelections {
		out.TableSelections[id] = selection
	}
	for key, binding := range in.Bindings {
		binding.Args = append(json.RawMessage(nil), binding.Args...)
		out.Bindings[key] = binding
	}
	return out
}

func (c *Client) Snapshot() engine.PresentationSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return clonePresentation(c.snapshot)
}

func (c *Client) ClientID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientID
}

func (c *Client) Updates() <-chan struct{} { return c.updates }

func (c *Client) Err() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastErr
}

// ConnectionError reports only fatal transport/codec failure. Recoverable
// daemon-side diagnostics (for example a stale publication ACK) do not make
// the semantic controller disconnected.
func (c *Client) ConnectionError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.connectionErr
}

func (c *Client) signalUpdate() {
	select {
	case c.updates <- struct{}{}:
	default:
	}
}

func (c *Client) readLoop() {
	for {
		msg, perr, err := c.reader.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				c.failAll(NewError("ipc.connection_closed", "daemon connection closed"))
			} else {
				c.failAll(NewError("ipc.connection_closed", err.Error()))
			}
			c.finish()
			return
		}
		if perr != nil {
			c.failAll(perr)
			c.finish()
			return
		}
		switch msg.Kind {
		case KindSnapshot:
			if msg.Snapshot == nil {
				c.failAll(NewError("ipc.invalid_message", "snapshot payload missing"))
				c.finish()
				return
			}
			c.mu.Lock()
			c.snapshot = clonePresentation(msg.Snapshot.Presentation)
			c.mu.Unlock()
			c.signalUpdate()
			if msg.RequestID != "" {
				c.resolve(msg.RequestID, nil)
			}
		case KindError:
			if msg.Error == nil {
				c.failAll(NewError("ipc.invalid_message", "error payload missing"))
				c.finish()
				return
			}
			if msg.RequestID != "" {
				c.resolve(msg.RequestID, msg.Error)
			} else {
				c.mu.Lock()
				c.lastErr = msg.Error
				c.mu.Unlock()
				c.signalUpdate()
			}
		default:
			c.failAll(NewError("ipc.invalid_message", fmt.Sprintf("unexpected daemon message %q", msg.Kind)))
			c.finish()
			return
		}
	}
}

func (c *Client) resolve(id string, perr *Error) {
	c.pendingMu.Lock()
	ch := c.pending[id]
	if ch != nil {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	if ch != nil {
		ch <- clientResponse{err: perr}
	}
}

func (c *Client) failAll(perr *Error) {
	c.mu.Lock()
	c.lastErr = perr
	c.connectionErr = perr
	c.mu.Unlock()
	c.pendingMu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan clientResponse)
	c.pendingMu.Unlock()
	for _, ch := range pending {
		ch <- clientResponse{err: perr}
	}
	c.signalUpdate()
}

func (c *Client) finish() {
	c.once.Do(func() {
		_ = c.conn.Close()
		close(c.closed)
	})
}

func (c *Client) Close() error {
	select {
	case <-c.closed:
		return nil
	default:
	}
	_ = c.writer.Write(Message{V: Version, Kind: KindDetach})
	c.finish()
	return nil
}

func (c *Client) request(in Interaction) error {
	id := fmt.Sprintf("r-%d", c.nextReq.Add(1))
	response := make(chan clientResponse, 1)
	c.pendingMu.Lock()
	select {
	case <-c.closed:
		c.pendingMu.Unlock()
		return NewError("ipc.connection_closed", "daemon connection is closed")
	default:
	}
	c.pending[id] = response
	c.pendingMu.Unlock()

	if err := c.writer.Write(Message{V: Version, Kind: KindInteraction, RequestID: id, Interaction: &in}); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return NewError("ipc.connection_closed", err.Error())
	}
	timer := time.NewTimer(defaultRequestTimeout)
	defer timer.Stop()
	select {
	case result := <-response:
		if result.err != nil {
			return result.err
		}
		return nil
	case <-timer.C:
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return NewError("ipc.request_timeout", "daemon interaction response timed out")
	case <-c.closed:
		return NewError("ipc.connection_closed", "daemon connection closed")
	}
}

func (c *Client) Focus(id string) error {
	return c.request(Interaction{Type: InteractionFocus, ID: id})
}
func (c *Client) SetInput(id, value string) error {
	return c.request(Interaction{Type: InteractionInputSet, ID: id, Value: value})
}
func (c *Client) Submit(id string) error {
	return c.request(Interaction{Type: InteractionInputSubmit, ID: id})
}
func (c *Client) MoveTableSelection(id string, delta int) error {
	return c.request(Interaction{Type: InteractionTableMove, ID: id, Delta: delta})
}
func (c *Client) ActivateTableSelection(id string) error {
	return c.request(Interaction{Type: InteractionTableActivate, ID: id})
}
func (c *Client) ActionKey(key string) error {
	return c.request(Interaction{Type: InteractionActionKey, Key: key})
}
func (c *Client) AcknowledgePublication(generation uint64) error {
	if generation == 0 {
		return NewError("ipc.invalid_message", "publication generation must be positive")
	}
	if err := c.writer.Write(Message{V: Version, Kind: KindFramePublished, PublicationGeneration: generation}); err != nil {
		return NewError("ipc.connection_closed", err.Error())
	}
	return nil
}
