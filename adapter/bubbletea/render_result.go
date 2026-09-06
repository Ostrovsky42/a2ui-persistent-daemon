package bubbletea

// ViewportMetrics describes the visual-line geometry used for manual scrolling.
// Offset is the effective offset after follow-tail and clamping are resolved.
type ViewportMetrics struct {
	TotalLines   int
	VisibleLines int
	MaxOffset    int
	Offset       int
}

// TableViewportMetrics describes renderer-resolved row-window geometry. It is
// presentation metadata only and never becomes A2UI Document/runtime state.
type TableViewportMetrics struct {
	TotalRows   int
	VisibleRows int
	MaxOffset   int
	Offset      int
}

// RenderResult is a pure rendering result. Metrics let the adapter manage local
// scrolling without duplicating renderer wrapping/layout calculations.
type RenderResult struct {
	Frame     string
	Viewports map[string]ViewportMetrics
	Tables    map[string]TableViewportMetrics
}
