package bubbletea

// ViewportState is Bubble Tea-local presentation state. It controls which
// rendered lines are visible but is never written to the A2UI document/runtime.
type ViewportState struct {
	Offset       int
	PinnedToTail bool
}

// TableViewportState is renderer-local table presentation state. Semantic row
// selection remains runtime-owned; this offset only selects the visible slice.
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
