package udp

import (
	"encoding/json"
	"fmt"

	"a2ui/protocol"
	"a2ui/wire"
)

const Magic = "A2UI"
const ChannelTelemetry = "telemetry"

type Datagram struct {
	Magic   string          `json:"magic"`
	V       int             `json:"v"`
	Session string          `json:"session"`
	Channel string          `json:"channel"`
	Seq     uint64          `json:"seq"`
	Payload json.RawMessage `json:"payload"`
}

func Encode(env protocol.Envelope, maxBytes int) ([]byte, *protocol.Error) {
	if env.V != protocol.Version {
		return nil, protocol.NewError("protocol.version_mismatch", fmt.Sprintf("unsupported version %d", env.V))
	}
	if env.Kind != protocol.KindTelemetry {
		return nil, protocol.NewError("udp.forbidden_kind", fmt.Sprintf("UDP v1 accepts telemetry only, got %q", env.Kind))
	}
	if env.Seq == 0 {
		return nil, protocol.NewError("udp.invalid_sequence", "UDP datagram sequence must be > 0")
	}
	if env.Session == "" {
		return nil, protocol.NewError("udp.invalid_session", "session id is required")
	}
	if _, err := wire.EncodeEnvelope(env); err != nil {
		if perr, ok := err.(*protocol.Error); ok {
			return nil, perr
		}
		return nil, protocol.NewError("udp.invalid_payload", err.Error())
	}
	d := Datagram{Magic: Magic, V: protocol.Version, Session: env.Session, Channel: ChannelTelemetry, Seq: env.Seq, Payload: append(json.RawMessage(nil), env.Payload...)}
	b, err := json.Marshal(d)
	if err != nil {
		return nil, protocol.NewError("udp.encode_failed", err.Error())
	}
	if maxBytes < 1 {
		maxBytes = protocol.DefaultLimits().MaxDatagramBytes
	}
	if len(b) > maxBytes {
		return nil, protocol.NewError("resource.datagram_too_large", fmt.Sprintf("datagram %d exceeds %d", len(b), maxBytes))
	}
	return b, nil
}

func Decode(b []byte, maxBytes int) (protocol.Envelope, *protocol.Error) {
	if maxBytes < 1 {
		maxBytes = protocol.DefaultLimits().MaxDatagramBytes
	}
	if len(b) > maxBytes {
		return protocol.Envelope{}, protocol.NewError("resource.datagram_too_large", "datagram exceeds configured limit")
	}
	var d Datagram
	if perr := wire.StrictUnmarshal(b, &d); perr != nil {
		return protocol.Envelope{}, perr
	}
	if d.Magic != Magic || d.V != protocol.Version || d.Channel != ChannelTelemetry {
		return protocol.Envelope{}, protocol.NewError("udp.invalid_header", "invalid A2UI UDP header")
	}
	if d.Session == "" || d.Seq == 0 {
		return protocol.Envelope{}, protocol.NewError("udp.invalid_header", "session and sequence are required")
	}
	env := protocol.Envelope{V: protocol.Version, Session: d.Session, Kind: protocol.KindTelemetry, Seq: d.Seq, Payload: append(json.RawMessage(nil), d.Payload...)}
	if _, err := wire.EncodeEnvelope(env); err != nil {
		if perr, ok := err.(*protocol.Error); ok {
			return protocol.Envelope{}, perr
		}
		return protocol.Envelope{}, protocol.NewError("udp.invalid_payload", err.Error())
	}
	return env, nil
}
