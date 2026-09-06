package daemon

import (
	"context"
	"errors"
	"io"
	"net"
	"time"

	"a2ui/ipc"
)

const (
	handshakeTimeout   = 5 * time.Second
	clientWriteTimeout = 2 * time.Second
)

type clientRead struct {
	msg  ipc.Message
	perr *ipc.Error
	err  error
}

// Serve accepts at most one interactive client at a time. Additional clients
// receive ipc.client_busy without disturbing the active lease.
func (d *Daemon) Serve(ctx context.Context, ln net.Listener) error {
	if ln == nil {
		return errors.New("nil IPC listener")
	}
	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
			d.closeActiveConn()
		case <-stop:
		}
	}()
	defer close(stop)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go d.handleClient(ctx, conn)
	}
}

func writeClientMessage(conn net.Conn, writer *ipc.Writer, msg ipc.Message) error {
	_ = conn.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
	err := writer.Write(msg)
	_ = conn.SetWriteDeadline(time.Time{})
	return err
}

func (d *Daemon) handleClient(parent context.Context, conn net.Conn) {
	ctx, cancel := context.WithCancel(parent)
	clientID := ""
	defer func() {
		cancel()
		if clientID != "" {
			d.clearActiveConn(conn)
			d.lease.release(clientID)
		}
		_ = conn.Close()
	}()

	reader := ipc.NewReader(conn, ipc.DefaultMaxMessageBytes)
	writer := ipc.NewWriter(conn)
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	msg, perr, err := reader.Next()
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil || perr != nil || msg.Kind != ipc.KindHello {
		if perr == nil {
			if err != nil && !errors.Is(err, io.EOF) {
				perr = ipc.NewError("ipc.connection_closed", err.Error())
			} else if msg.Kind != ipc.KindHello {
				perr = ipc.NewError("ipc.invalid_message", "first IPC record must be hello")
			}
		}
		if perr != nil {
			_ = writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindError, Error: perr})
		}
		return
	}

	clientID, perr = d.lease.acquire()
	if perr != nil {
		_ = writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindError, Error: perr})
		return
	}
	d.setActiveConn(conn)

	if err := writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindHelloAck, ClientID: clientID, Interactive: true}); err != nil {
		return
	}
	if err := d.writeSnapshot(conn, writer, ""); err != nil {
		return
	}

	reads := make(chan clientRead, 1)
	go func() {
		for {
			msg, perr, err := reader.Next()
			select {
			case reads <- clientRead{msg: msg, perr: perr, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil || perr != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.updates:
			if err := d.writeSnapshot(conn, writer, ""); err != nil {
				return
			}
		case read := <-reads:
			if read.err != nil {
				return
			}
			if read.perr != nil {
				_ = writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindError, Error: read.perr})
				return
			}
			msg := read.msg
			switch msg.Kind {
			case ipc.KindDetach:
				// Release semantic ownership before acknowledging detach so a successful
				// Client.Close guarantees immediate reattach cannot observe client_busy.
				if clientID != "" {
					d.clearActiveConn(conn)
					d.lease.release(clientID)
					clientID = ""
				}
				if msg.RequestID != "" {
					if err := writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindDetachAck, RequestID: msg.RequestID}); err != nil {
						return
					}
				}
				return
			case ipc.KindInteraction:
				if ierr := d.handleInteraction(ctx, msg.Interaction); ierr != nil {
					_ = writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindError, RequestID: msg.RequestID, Error: ierr})
					continue
				}
				d.signalEvents()
				if err := d.writeSnapshot(conn, writer, msg.RequestID); err != nil {
					return
				}
			case ipc.KindFramePublished:
				if ierr := d.ackPublication(msg.PublicationGeneration); ierr != nil {
					_ = writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindError, RequestID: msg.RequestID, Error: ierr})
				} else {
					d.signalEvents()
				}
			default:
				_ = writeClientMessage(conn, writer, ipc.Message{V: ipc.Version, Kind: ipc.KindError, RequestID: msg.RequestID, Error: ipc.NewError("ipc.invalid_message", "message is not valid after attach")})
			}
		}
	}
}

func (d *Daemon) writeSnapshot(conn net.Conn, writer *ipc.Writer, requestID string) error {
	snapshot := d.Engine.PresentationSnapshot()
	return writeClientMessage(conn, writer, ipc.Message{
		V:         ipc.Version,
		Kind:      ipc.KindSnapshot,
		RequestID: requestID,
		Snapshot:  &ipc.Snapshot{Presentation: snapshot},
	})
}
