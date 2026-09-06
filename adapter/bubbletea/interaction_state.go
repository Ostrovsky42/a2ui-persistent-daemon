package bubbletea

// ViewportState is Bubble Tea-local user-controlled scroll state. Offset is
// changed by viewport navigation keys and PinnedToTail carries follow-tail
// policy; neither value is written to the A2UI document/runtime.
type ViewportState struct {
	Offset       int
	PinnedToTail bool
}

// TableViewportState is intentionally a different state machine from
// ViewportState. It is not independently scrollable and has no pin/follow-tail
// policy. Offset only caches the top rendered row chosen from runtime-owned
// semantic selection plus current geometry, so adjacent selection changes can
// avoid needless visual jumps. Reconciliation always clamps/overwrites it.
type TableViewportState struct {
	Offset int
}

// InteractionState owns only adapter-local interaction mechanics. Semantic
// state such as focus, input values and table selection stays in runtime.State.
type InteractionState struct {
	InputCarets    map[string]int
	Viewports      map[string]ViewportState
	TableViewports map[string]TableViewportState
}

func newInteractionState() InteractionState {
	return InteractionState{
		InputCarets:    make(map[string]int),
		Viewports:      make(map[string]ViewportState),
		TableViewports: make(map[string]TableViewportState),
	}
}

func (s InteractionState) clone() InteractionState {
	out := newInteractionState()
	for id, caret := range s.InputCarets {
		out.InputCarets[id] = caret
	}
	for id, viewport := range s.Viewports {
		out.Viewports[id] = viewport
	}
	for id, viewport := range s.TableViewports {
		out.TableViewports[id] = viewport
	}
	return out
}
