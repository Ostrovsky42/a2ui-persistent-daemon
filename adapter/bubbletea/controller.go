package bubbletea

import (
	"context"
	"encoding/json"

	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

// SemanticController is the renderer/controller boundary used by Bubble Tea.
// Implementations may be in-process (Engine-backed) or remote (IPC-backed),
// but all semantic authority remains behind this interface. Physical key input
// is resolved to semantic interactions inside the renderer before crossing it.
type SemanticController interface {
	Snapshot() engine.PresentationSnapshot
	Focus(id string) error
	SetInput(id, value string) error
	Submit(id string) error
	MoveTableSelection(id string, delta int) error
	ActivateTableSelection(id string) error
	InvokeAction(id, action string, args json.RawMessage) error
	AcknowledgePublication(generation uint64) error
}

type localSemanticController struct {
	eng *engine.Engine
}

func (c localSemanticController) Snapshot() engine.PresentationSnapshot {
	if c.eng == nil {
		return engine.PresentationSnapshot{}
	}
	return c.eng.PresentationSnapshot()
}

func (c localSemanticController) Focus(id string) error {
	return controllerError(c.eng.Focus(id))
}

func (c localSemanticController) SetInput(id, value string) error {
	return controllerError(c.eng.SetInput(id, value))
}

func (c localSemanticController) Submit(id string) error {
	return controllerError(c.eng.Submit(id))
}

func (c localSemanticController) MoveTableSelection(id string, delta int) error {
	return controllerError(c.eng.MoveTableSelection(id, delta))
}

func (c localSemanticController) ActivateTableSelection(id string) error {
	return controllerError(c.eng.ActivateTableSelection(id))
}

func (c localSemanticController) InvokeAction(id, action string, args json.RawMessage) error {
	return controllerError(c.eng.InvokeAction(context.Background(), id, action, args))
}

func (c localSemanticController) AcknowledgePublication(generation uint64) error {
	_, err := c.eng.PublishGeneration(generation)
	return controllerError(err)
}

func controllerError(err *protocol.Error) error {
	if err == nil {
		return nil
	}
	return err
}

type controllerErrorSource interface {
	ConnectionError() error
}

func semanticControllerError(controller SemanticController) error {
	if source, ok := controller.(controllerErrorSource); ok {
		return source.ConnectionError()
	}
	return nil
}
