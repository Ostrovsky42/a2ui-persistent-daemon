package daemon

import (
	"context"

	"a2ui/ipc"
	"a2ui/protocol"
)

func semanticIPCError(perr *protocol.Error) *ipc.Error {
	if perr == nil {
		return nil
	}
	return &ipc.Error{Code: perr.Code, Message: perr.Message, Recoverable: perr.Recoverable}
}

func (d *Daemon) handleInteraction(ctx context.Context, in *ipc.Interaction) *ipc.Error {
	if ierr := ipc.ValidateInteraction(in); ierr != nil {
		return ierr
	}
	var perr *protocol.Error
	switch in.Type {
	case ipc.InteractionFocus:
		perr = d.Engine.Focus(in.ID)
	case ipc.InteractionInputSet:
		perr = d.Engine.SetInput(in.ID, in.Value)
	case ipc.InteractionInputSubmit:
		perr = d.Engine.Submit(in.ID)
	case ipc.InteractionTableMove:
		perr = d.Engine.MoveTableSelection(in.ID, in.Delta)
	case ipc.InteractionTableActivate:
		perr = d.Engine.ActivateTableSelection(in.ID)
	case ipc.InteractionActionKey:
		perr = d.Engine.HandleKey(ctx, in.Key)
	default:
		return ipc.NewError("ipc.invalid_interaction", "unsupported interaction")
	}
	return semanticIPCError(perr)
}
