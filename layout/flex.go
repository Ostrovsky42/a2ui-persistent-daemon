package layout

// FlexConstraint is a renderer-neutral sizing hint for a child in a row.
// Zero values mean "unspecified" except Grow=0, which means the child does not
// claim spare width.
type FlexConstraint struct {
	Grow     int
	Basis    int
	MinWidth int
	MaxWidth int
}

// AllocateRow allocates widths for row children. The returned fits flag is
// false only when the declared minimum widths cannot fit in available space.
// Widths are always deterministic and at least one cell for visible children.
func AllocateRow(available, gap int, children []FlexConstraint) ([]int, bool) {
	n := len(children)
	if n == 0 {
		return nil, true
	}
	if gap < 0 {
		gap = 0
	}
	content := available - GapExtent(Row, gap, n)
	if content < 0 {
		content = 0
	}

	widths := make([]int, n)
	mins := make([]int, n)
	for i, c := range children {
		minW := c.MinWidth
		if minW < 1 {
			minW = 1
		}
		mins[i] = minW
		w := c.Basis
		if w < minW {
			w = minW
		}
		if c.MaxWidth > 0 && w > c.MaxWidth {
			w = c.MaxWidth
		}
		if w < 1 {
			w = 1
		}
		widths[i] = w
	}

	minSum := 0
	for _, w := range mins {
		minSum += w
	}
	if minSum > content {
		return widthsFromMinimums(mins), false
	}

	// Shrink oversized bases toward declared minima, from left to right in
	// round-robin order so the result is stable and balanced.
	for sumInts(widths) > content {
		changed := false
		for i := range widths {
			if sumInts(widths) <= content {
				break
			}
			if widths[i] > mins[i] {
				widths[i]--
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	spare := content - sumInts(widths)
	if spare <= 0 {
		return widths, true
	}

	// Repeated proportional passes allow capped children to hand unused width
	// to uncapped siblings. Fractional remainders decide leftover cells; ties
	// are resolved by lower child index.
	for spare > 0 {
		totalGrow := 0
		active := make([]int, 0, n)
		for i, c := range children {
			if c.Grow > 0 && (c.MaxWidth <= 0 || widths[i] < c.MaxWidth) {
				totalGrow += c.Grow
				active = append(active, i)
			}
		}
		if totalGrow == 0 {
			break
		}

		type fraction struct{ idx, rem int }
		fractions := make([]fraction, 0, len(active))
		grants := make([]int, n)
		granted := 0
		for _, i := range active {
			c := children[i]
			num := spare * c.Grow
			share := num / totalGrow
			fractions = append(fractions, fraction{idx: i, rem: num % totalGrow})
			if c.MaxWidth > 0 && widths[i]+share > c.MaxWidth {
				share = c.MaxWidth - widths[i]
			}
			if share > 0 {
				grants[i] = share
				granted += share
			}
		}

		left := spare - granted
		for left > 0 {
			best := -1
			bestRem := -1
			for _, f := range fractions {
				i := f.idx
				c := children[i]
				if c.MaxWidth > 0 && widths[i]+grants[i] >= c.MaxWidth {
					continue
				}
				if f.rem > bestRem {
					best, bestRem = i, f.rem
				}
			}
			if best < 0 {
				break
			}
			grants[best]++
			left--
			// The same fractional remainder should not receive the next leftover
			// until peers have had a chance in this pass.
			for j := range fractions {
				if fractions[j].idx == best {
					fractions[j].rem = -1
					break
				}
			}
		}

		granted = 0
		for i, g := range grants {
			if g > 0 {
				widths[i] += g
				granted += g
			}
		}
		if granted == 0 {
			break
		}
		spare -= granted
	}

	return widths, true
}

func widthsFromMinimums(mins []int) []int {
	out := make([]int, len(mins))
	copy(out, mins)
	return out
}

func sumInts(values []int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}
