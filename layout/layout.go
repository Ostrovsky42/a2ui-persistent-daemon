package layout

type Direction string

const (
	Column Direction = "col"
	Row    Direction = "row"
)

type Border string

const (
	BorderNone    Border = "none"
	BorderRounded Border = "rounded"
	BorderNormal  Border = "normal"
)

type Box struct {
	Dir     Direction
	Padding int
	Gap     int
	Border  Border
}
type Metrics struct {
	OuterWidth    int
	OuterHeight   int
	ContentWidth  int
	ContentHeight int
	BorderInset   int
	PaddingInset  int
}

func ComputeBox(width, height int, b Box) Metrics {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if b.Padding < 0 {
		b.Padding = 0
	}
	border := 0
	if b.Border != BorderNone && b.Border != "" {
		border = 1
	}
	cw := width - 2*border - 2*b.Padding
	ch := height - 2*border - 2*b.Padding
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	return Metrics{OuterWidth: width, OuterHeight: height, ContentWidth: cw, ContentHeight: ch, BorderInset: border, PaddingInset: b.Padding}
}

func GapExtent(_ Direction, gap, children int) int {
	if gap <= 0 || children <= 1 {
		return 0
	}
	return gap * (children - 1)
}
func ClampViewportHeight(requested, available int) int {
	if available < 1 {
		available = 1
	}
	if requested < 1 {
		requested = 1
	}
	if requested > available {
		return available
	}
	return requested
}
