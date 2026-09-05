package bubbletea

import (
	"context"

	"a2ui/engine"
	"a2ui/protocol"
)

// SemanticController is the renderer/controller boundary used by Bubble Tea.
// Implementations may be in-process (Engine-backed) or remote (IPC-backed),
// but all semantic authority remains behind this interface.
type SemanticController interface {
	Snapshot() engine.PresentationSnapshot
	Focus(id string) error
	SetInput(id, value string) error
	Submit(id string) error
	MoveTableSelection(id string, delta int) error
	ActivateTableSelection(id string) error
	ActionKey(key string) error
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

func (c localSemanticController) ActionKey(key string) error {
	return controllerError(c.eng.HandleKey(context.Background(), key))
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
