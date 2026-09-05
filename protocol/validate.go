package protocol

import (
	"encoding/json"
	"fmt"
)

func ValidateOperation(op Operation, limits Limits) *Error {
	if op.V != Version {
		return NewError("protocol.version_mismatch", fmt.Sprintf("unsupported version %d", op.V))
	}
	if op.Seq < 0 {
		return NewError("protocol.invalid_sequence", "operation sequence must be non-negative")
	}
	switch op.Op {
	case OpUpsert:
		if op.ID == "" {
			return NewError("schema.missing_field", "upsert.id is required")
		}
		if !op.Type.Valid() {
			return NewError("schema.unknown_type", fmt.Sprintf("unknown node type %q", op.Type))
		}
		if len(op.Props) > limits.MaxMessageBytes {
			return NewError("resource.message_too_large", "props exceed message limit")
		}
		if _, _, e := NormalizeProps(op.Type, op.Props, true); e != nil {
			return e
		}
	case OpProps:
		if len(op.Props) > limits.MaxMessageBytes {
			return NewError("resource.message_too_large", "props exceed message limit")
		}
		if op.ID == "" {
			return NewError("schema.missing_field", "props.id is required")
		}
		if len(op.Props) == 0 {
			return NewError("schema.missing_field", "props.props is required")
		}
		var x map[string]json.RawMessage
		if json.Unmarshal(op.Props, &x) != nil || x == nil {
			return NewError("schema.invalid_props", "props must be object")
		}
	case OpText:
		if op.ID == "" {
			return NewError("schema.missing_field", "text.id is required")
		}
		if len(op.Text) > limits.MaxTextBytesPerNode {
			return NewError("resource.text_too_large", "text chunk too large")
		}
	case OpRemove, OpFocus:
		if op.ID == "" {
			return NewError("schema.missing_field", string(op.Op)+".id is required")
		}
	case OpCommit:
	default:
		return NewError("protocol.unknown_op", fmt.Sprintf("unknown op %q", op.Op))
	}
	return nil
}

func ValidateLimits(limits Limits) *Error {
	fields := []struct {
		name  string
		value int
	}{
		{"max_message_bytes", limits.MaxMessageBytes},
		{"max_document_bytes", limits.MaxDocumentBytes},
		{"max_nodes", limits.MaxNodes},
		{"max_depth", limits.MaxDepth},
		{"max_children", limits.MaxChildren},
		{"max_text_bytes_per_node", limits.MaxTextBytesPerNode},
		{"max_total_text_bytes", limits.MaxTotalTextBytes},
		{"max_table_rows", limits.MaxTableRows},
		{"max_table_columns", limits.MaxTableColumns},
		{"max_pending_events", limits.MaxPendingEvents},
		{"max_pending_mutations", limits.MaxPendingMutations},
		{"max_inflight_actions", limits.MaxInflightActions},
		{"max_datagram_bytes", limits.MaxDatagramBytes},
	}
	for _, field := range fields {
		if field.value < 1 {
			err := NewError("schema.invalid_limits", fmt.Sprintf("%s must be >= 1", field.name))
			err.Path = "/limits/" + field.name
			return err
		}
	}
	return nil
}

func ValidateHello(h Hello) *Error {
	if len(h.Versions) == 0 {
		return NewError("schema.missing_field", "hello.versions is required")
	}
	seenVersions := make(map[int]struct{}, len(h.Versions))
	for _, version := range h.Versions {
		if version < 1 {
			return NewError("schema.invalid_hello", "hello versions must be positive integers")
		}
		if _, exists := seenVersions[version]; exists {
			return NewError("schema.invalid_hello", "hello versions must be unique")
		}
		seenVersions[version] = struct{}{}
	}
	seenFeatures := make(map[string]struct{}, len(h.Features))
	for _, feature := range h.Features {
		if feature == "" {
			return NewError("schema.invalid_hello", "hello features must be non-empty")
		}
		if _, exists := seenFeatures[feature]; exists {
			return NewError("schema.invalid_hello", "hello features must be unique")
		}
		seenFeatures[feature] = struct{}{}
	}
	return nil
}

func ValidateHelloAck(ack HelloAck) *Error {
	if ack.Version != Version {
		return NewError("protocol.version_mismatch", fmt.Sprintf("unsupported negotiated version %d", ack.Version))
	}
	seenFeatures := make(map[string]struct{}, len(ack.Features))
	for _, feature := range ack.Features {
		if feature == "" {
			return NewError("schema.invalid_hello_ack", "hello_ack features must be non-empty")
		}
		if _, exists := seenFeatures[feature]; exists {
			return NewError("schema.invalid_hello_ack", "hello_ack features must be unique")
		}
		seenFeatures[feature] = struct{}{}
	}
	seenComponents := make(map[NodeType]struct{}, len(ack.Components))
	for _, component := range ack.Components {
		if !component.Valid() {
			return NewError("schema.unknown_type", fmt.Sprintf("unknown component %q", component))
		}
		if _, exists := seenComponents[component]; exists {
			return NewError("schema.invalid_hello_ack", "hello_ack components must be unique")
		}
		seenComponents[component] = struct{}{}
	}
	return ValidateLimits(ack.Limits)
}
