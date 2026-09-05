package daemon

import "a2ui/ipc"

func (d *Daemon) ackPublication(generation uint64) *ipc.Error {
	current, pending := d.Engine.PublicationGeneration()
	if generation != current {
		return ipc.NewError("ipc.stale_publication", "frame acknowledgement does not match the current publication generation")
	}
	if !pending {
		// Duplicate acknowledgement for an already-published current generation.
		return nil
	}
	published, perr := d.Engine.PublishGeneration(generation)
	if perr != nil {
		return semanticIPCError(perr)
	}
	if !published {
		// The generation changed between the status read and the conditional
		// publish, so this acknowledgement became stale under us.
		return ipc.NewError("ipc.stale_publication", "publication generation changed before acknowledgement")
	}
	return nil
}
