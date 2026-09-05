package daemon

import (
	"encoding/json"
	"fmt"

	"a2ui/protocol"
	"a2ui/wire"
)

// HandleEnvelope processes the public Agent-facing A2UI protocol while keeping
// visual publication owned by the attached renderer client. It deliberately
// never calls Engine.Publish().
func (d *Daemon) HandleEnvelope(env protocol.Envelope) (*protocol.Envelope, *protocol.Error) {
	d.agentMu.Lock()
	defer d.agentMu.Unlock()

	if env.Session != "" && env.Session != d.Session.ID() {
		return nil, protocol.NewError("protocol.session_mismatch", fmt.Sprintf("expected session %q, got %q", d.Session.ID(), env.Session))
	}

	switch env.Kind {
	case protocol.KindHello:
		var h protocol.Hello
		if perr := wire.StrictUnmarshal(env.Payload, &h); perr != nil {
			return nil, perr
		}
		ack, perr := d.Session.Negotiate(h)
		if perr != nil {
			return nil, perr
		}
		payload, err := json.Marshal(ack)
		if err != nil {
			return nil, protocol.NewError("transport.encode_error", err.Error())
		}
		return &protocol.Envelope{V: protocol.Version, Session: d.Session.ID(), Kind: protocol.KindHelloAck, Payload: payload}, nil

	case protocol.KindOperation:
		duplicate, perr := d.Session.AcceptMutation(env.Seq)
		if perr != nil {
			return nil, perr
		}
		if duplicate {
			return nil, nil
		}
		var op protocol.Operation
		if perr := wire.StrictUnmarshal(env.Payload, &op); perr != nil {
			return nil, perr
		}
		if op.Seq != int64(env.Seq) {
			return nil, protocol.NewError("protocol.sequence_mismatch", fmt.Sprintf("envelope seq %d != operation seq %d", env.Seq, op.Seq))
		}
		if perr := d.Engine.Apply(op); perr != nil {
			return nil, perr
		}
		d.signalSnapshot()
		return nil, nil

	case protocol.KindTelemetry:
		return nil, nil
	default:
		return nil, protocol.NewError("protocol.unsupported_kind", fmt.Sprintf("unsupported envelope kind %q", env.Kind))
	}
}

// DrainEvents transfers the daemon Engine's already-classified semantic events
// to whichever Agent transport owns this Daemon instance.
func (d *Daemon) DrainEvents() []protocol.Event {
	var out []protocol.Event
	for {
		ev, ok := d.Engine.NextEvent()
		if !ok {
			return out
		}
		out = append(out, ev)
	}
}
