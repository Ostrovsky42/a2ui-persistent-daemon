package wire

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"a2ui/protocol"
)

func DecodeRecord(raw []byte) (protocol.Envelope, *protocol.Error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return protocol.Envelope{}, protocol.NewError("wire.empty", "empty record")
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return protocol.Envelope{}, duplicateOrMalformed(err)
	}
	probe, perr := objectProbe(raw)
	if perr != nil {
		return protocol.Envelope{}, perr
	}

	if _, ok := probe["kind"]; ok {
		if _, ok := probe["v"]; !ok {
			return protocol.Envelope{}, protocol.NewError("wire.missing_field", "envelope requires v")
		}
		var env protocol.Envelope
		if err := strictDecode(raw, &env); err != nil {
			return protocol.Envelope{}, wireDecodeError(err)
		}
		if env.V != protocol.Version {
			return protocol.Envelope{}, protocol.NewError("protocol.version_mismatch", fmt.Sprintf("unsupported version %d", env.V))
		}
		if perr := validateEnvelopePayload(env, probe); perr != nil {
			return protocol.Envelope{}, perr
		}
		return env, nil
	}

	if _, ok := probe["op"]; ok {
		op, perr := decodeLegacyOperation(raw)
		if perr != nil {
			return protocol.Envelope{}, perr
		}
		payload := append(json.RawMessage(nil), raw...)
		return protocol.Envelope{V: op.V, Kind: protocol.KindOperation, Seq: uint64(op.Seq), Payload: payload}, nil
	}

	if _, ok := probe["ev"]; ok {
		ev, perr := decodeDirectEvent(raw)
		if perr != nil {
			return protocol.Envelope{}, perr
		}
		payload := append(json.RawMessage(nil), raw...)
		return protocol.Envelope{V: ev.V, Kind: protocol.KindEvent, Seq: ev.Seq, Payload: payload}, nil
	}

	return protocol.Envelope{}, protocol.NewError("wire.malformed", "record is neither envelope nor legacy operation/event")
}

func validateEnvelopePayload(env protocol.Envelope, probe map[string]json.RawMessage) *protocol.Error {
	if _, ok := probe["payload"]; !ok {
		return protocol.NewError("wire.missing_field", fmt.Sprintf("%s envelope requires payload", env.Kind))
	}
	switch env.Kind {
	case protocol.KindHello:
		var h protocol.Hello
		if perr := StrictUnmarshal(env.Payload, &h); perr != nil {
			return perr
		}
		if perr := protocol.ValidateHello(h); perr != nil {
			return perr
		}
	case protocol.KindHelloAck:
		var ack protocol.HelloAck
		if perr := StrictUnmarshal(env.Payload, &ack); perr != nil {
			return perr
		}
		if perr := protocol.ValidateHelloAck(ack); perr != nil {
			return perr
		}
	case protocol.KindOperation:
		if _, ok := probe["seq"]; !ok {
			return protocol.NewError("wire.missing_field", "operation envelope requires seq")
		}
		op, perr := decodeLegacyOperation(env.Payload)
		if perr != nil {
			return perr
		}
		if uint64(op.Seq) != env.Seq {
			return protocol.NewError("protocol.sequence_mismatch", fmt.Sprintf("envelope seq %d differs from operation seq %d", env.Seq, op.Seq))
		}
	case protocol.KindEvent:
		if _, ok := probe["seq"]; !ok {
			return protocol.NewError("wire.missing_field", "event envelope requires seq")
		}
		ev, perr := decodeDirectEvent(env.Payload)
		if perr != nil {
			return perr
		}
		if ev.Seq != env.Seq {
			return protocol.NewError("protocol.sequence_mismatch", fmt.Sprintf("envelope seq %d differs from event seq %d", env.Seq, ev.Seq))
		}
	case protocol.KindTelemetry:
		if _, ok := probe["seq"]; !ok {
			return protocol.NewError("wire.missing_field", "telemetry envelope requires seq")
		}
		var payload map[string]json.RawMessage
		if perr := StrictUnmarshal(env.Payload, &payload); perr != nil {
			return perr
		}
		if payload == nil {
			return protocol.NewError("wire.malformed", "telemetry payload must be a JSON object")
		}
	default:
		return protocol.NewError("wire.unknown_kind", fmt.Sprintf("unknown kind %q", env.Kind))
	}
	return nil
}

func decodeLegacyOperation(raw []byte) (protocol.Operation, *protocol.Error) {
	probe, perr := objectProbe(raw)
	if perr != nil {
		return protocol.Operation{}, perr
	}
	for _, field := range []string{"v", "seq", "op"} {
		if _, ok := probe[field]; !ok {
			return protocol.Operation{}, protocol.NewError("wire.missing_field", fmt.Sprintf("legacy operation requires %s", field))
		}
	}
	var op protocol.Operation
	if err := strictDecode(raw, &op); err != nil {
		return protocol.Operation{}, wireDecodeError(err)
	}
	if op.V != protocol.Version {
		return protocol.Operation{}, protocol.NewError("protocol.version_mismatch", fmt.Sprintf("unsupported version %d", op.V))
	}
	if op.Seq < 0 {
		return protocol.Operation{}, protocol.NewError("protocol.invalid_sequence", "legacy seq must be >= 0")
	}
	return op, nil
}

func decodeDirectEvent(raw []byte) (protocol.Event, *protocol.Error) {
	probe, perr := objectProbe(raw)
	if perr != nil {
		return protocol.Event{}, perr
	}
	for _, field := range []string{"v", "seq", "ev"} {
		if _, ok := probe[field]; !ok {
			return protocol.Event{}, protocol.NewError("wire.missing_field", fmt.Sprintf("event requires %s", field))
		}
	}
	var ev protocol.Event
	if err := strictDecode(raw, &ev); err != nil {
		return protocol.Event{}, wireDecodeError(err)
	}
	if ev.V != protocol.Version {
		return protocol.Event{}, protocol.NewError("protocol.version_mismatch", fmt.Sprintf("unsupported version %d", ev.V))
	}
	if ev.Ev == "" {
		return protocol.Event{}, protocol.NewError("wire.missing_field", "event requires ev")
	}
	return ev, nil
}

func objectProbe(raw []byte) (map[string]json.RawMessage, *protocol.Error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		return nil, protocol.NewError("wire.malformed", err.Error())
	}
	if probe == nil {
		return nil, protocol.NewError("wire.malformed", "JSON record must be an object")
	}
	return probe, nil
}

func rejectDuplicateKeys(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parseValue func() error
	parseValue = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]struct{}{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key, ok := keyTok.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON object key %q", key)
				}
				seen[key] = struct{}{}
				if err := parseValue(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := parseValue(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		default:
			return fmt.Errorf("unexpected delimiter %q", delim)
		}
	}
	if err := parseValue(); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values in one record")
		}
		return err
	}
	return nil
}

func strictDecode(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values in one record")
		}
		return err
	}
	return nil
}

func duplicateOrMalformed(err error) *protocol.Error {
	if strings.Contains(err.Error(), "duplicate JSON object key") {
		return protocol.NewError("wire.duplicate_key", err.Error())
	}
	return protocol.NewError("wire.malformed", err.Error())
}

func wireDecodeError(err error) *protocol.Error {
	if strings.Contains(err.Error(), "unknown field") {
		return protocol.NewError("wire.unknown_field", err.Error())
	}
	return protocol.NewError("wire.malformed", err.Error())
}

func EncodeEnvelope(env protocol.Envelope) ([]byte, error) {
	b, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	if _, perr := DecodeRecord(b); perr != nil {
		return nil, perr
	}
	return b, nil
}

// StrictUnmarshal decodes one complete JSON value, rejecting duplicate object
// keys, unknown struct fields, trailing values, and empty input. It is shared by
// non-NDJSON transport profiles so all zero-trust JSON entry points have the
// same ambiguity rules.
func StrictUnmarshal(raw []byte, dst any) *protocol.Error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return protocol.NewError("wire.empty", "empty JSON value")
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return duplicateOrMalformed(err)
	}
	if err := strictDecode(raw, dst); err != nil {
		return wireDecodeError(err)
	}
	return nil
}
