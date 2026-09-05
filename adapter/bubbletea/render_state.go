package bubbletea

// RenderState contains renderer-local ephemeral state. None of these fields
// belong to the A2UI document or runtime projection.
type RenderState struct {
	AnimationPhase uint64
	CursorVisible  bool
}
