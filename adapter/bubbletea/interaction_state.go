package bubbletea

// ViewportState is Bubble Tea-local presentation state. It controls which
// rendered lines are visible but is never written to the A2UI document/runtime.
type ViewportState struct {
	Offset       int
	PinnedToTail bool
}

// InteractionState owns only adapter-local interaction mechanics. Semantic
// state such as focus, input values and table selection stays in runtime.State.
type InteractionState struct {
	InputCarets map[string]int
	Viewports   map[string]ViewportState
}

func newInteractionState() InteractionState {
	return InteractionState{
		InputCarets: make(map[string]int),
		Viewports:   make(map[string]ViewportState),
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
	return out
}
