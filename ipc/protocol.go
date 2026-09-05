package ipc

import (
	"fmt"

	"a2ui/engine"
)

const Version = 1

type Kind string

const (
	KindHello          Kind = "hello"
	KindHelloAck       Kind = "hello_ack"
	KindInteraction    Kind = "interaction"
	KindFramePublished Kind = "frame_published"
	KindDetach         Kind = "detach"
	KindDetachAck      Kind = "detach_ack"
	KindSnapshot       Kind = "snapshot"
	KindError          Kind = "error"
)

type InteractionType string

const (
	InteractionFocus         InteractionType = "focus"
	InteractionInputSet      InteractionType = "input_set"
	InteractionInputSubmit   InteractionType = "input_submit"
	InteractionTableMove     InteractionType = "table_move"
	InteractionTableActivate InteractionType = "table_activate"
	InteractionActionKey     InteractionType = "action_key"
)

type Interaction struct {
	Type  InteractionType `json:"type"`
	ID    string          `json:"id,omitempty"`
	Value string          `json:"value,omitempty"`
	Delta int             `json:"delta,omitempty"`
	Key   string          `json:"key,omitempty"`
}

type Snapshot struct {
	Presentation engine.PresentationSnapshot `json:"presentation"`
}

type Error struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func NewError(code, message string) *Error {
	return &Error{Code: code, Message: message, Recoverable: true}
}

// Message is the versioned local daemon/client control envelope. Fields are
// intentionally finite and kind-discriminated; this is not the public A2UI
// Agent protocol.
type Message struct {
	V                     int          `json:"v"`
	Kind                  Kind         `json:"kind"`
	RequestID             string       `json:"request_id,omitempty"`
	Client                string       `json:"client,omitempty"`
	ClientID              string       `json:"client_id,omitempty"`
	Interactive           bool         `json:"interactive,omitempty"`
	Interaction           *Interaction `json:"interaction,omitempty"`
	Snapshot              *Snapshot    `json:"snapshot,omitempty"`
	PublicationGeneration uint64       `json:"publication_generation,omitempty"`
	Error                 *Error       `json:"error,omitempty"`
}

func ValidateInteraction(in *Interaction) *Error {
	if in == nil {
		return NewError("ipc.invalid_interaction", "interaction payload is required")
	}
	switch in.Type {
	case InteractionFocus, InteractionInputSet, InteractionInputSubmit, InteractionTableMove, InteractionTableActivate:
		if in.ID == "" {
			return NewError("ipc.invalid_interaction", fmt.Sprintf("%s requires id", in.Type))
		}
	case InteractionActionKey:
		if in.Key == "" {
			return NewError("ipc.invalid_interaction", "action_key requires key")
		}
	default:
		return NewError("ipc.invalid_interaction", fmt.Sprintf("unknown interaction type %q", in.Type))
	}
	return nil
}

func validateMessage(m Message) *Error {
	if m.V != Version {
		return NewError("ipc.version_mismatch", fmt.Sprintf("unsupported IPC version %d", m.V))
	}
	switch m.Kind {
	case KindHello:
		if m.Client == "" {
			return NewError("ipc.invalid_message", "hello requires client")
		}
	case KindHelloAck:
		if m.ClientID == "" || !m.Interactive {
			return NewError("ipc.invalid_message", "hello_ack requires interactive client_id")
		}
	case KindInteraction:
		if err := ValidateInteraction(m.Interaction); err != nil {
			return err
		}
	case KindFramePublished:
		if m.PublicationGeneration == 0 {
			return NewError("ipc.invalid_message", "frame_published requires publication_generation")
		}
	case KindDetach:
		// No payload required. RequestID is optional for backward compatibility.
	case KindDetachAck:
		if m.RequestID == "" {
			return NewError("ipc.invalid_message", "detach_ack requires request_id")
		}
	case KindSnapshot:
		if m.Snapshot == nil {
			return NewError("ipc.invalid_message", "snapshot requires payload")
		}
		if _, ok := m.Snapshot.Presentation.Document.Nodes["root"]; !ok {
			return NewError("ipc.invalid_message", "snapshot document requires root")
		}
		if m.Snapshot.Presentation.PublicationPending && m.Snapshot.Presentation.PublicationGeneration == 0 {
			return NewError("ipc.invalid_message", "pending snapshot requires publication generation")
		}
	case KindError:
		if m.Error == nil || m.Error.Code == "" {
			return NewError("ipc.invalid_message", "error message requires error payload")
		}
	default:
		return NewError("ipc.invalid_message", fmt.Sprintf("unknown IPC message kind %q", m.Kind))
	}
	return nil
}
