package layout

import (
	"reflect"
	"testing"
)

func TestBoxModelAccountsForBorderAndPadding(t *testing.T) {
	m := ComputeBox(20, 10, Box{Padding: 2, Border: BorderNormal, Dir: Column, Gap: 1})
	if m.ContentWidth != 14 || m.ContentHeight != 4 {
		t.Fatalf("metrics=%+v", m)
	}
}

func TestGapAppliesToRowsAndColumns(t *testing.T) {
	if got := GapExtent(Row, 3, 4); got != 9 {
		t.Fatalf("row gap extent=%d", got)
	}
	if got := GapExtent(Column, 3, 4); got != 9 {
		t.Fatalf("col gap extent=%d", got)
	}
}

func TestViewportHeightClampsToAvailableTerminalSpace(t *testing.T) {
	if got := ClampViewportHeight(20, 7); got != 7 {
		t.Fatalf("got=%d", got)
	}
	if got := ClampViewportHeight(0, 7); got != 1 {
		t.Fatalf("got=%d", got)
	}
}

func TestTinyTerminalNeverProducesNegativeContentArea(t *testing.T) {
	m := ComputeBox(2, 1, Box{Padding: 3, Border: BorderNormal})
	if m.ContentWidth != 1 || m.ContentHeight != 1 {
		t.Fatalf("metrics=%+v", m)
	}
}

func TestAllocateRowFlexConstraints(t *testing.T) {
	tests := []struct {
		name      string
		available int
		gap       int
		children  []FlexConstraint
		want      []int
		fits      bool
	}{
		{"equal no grow", 21, 1, []FlexConstraint{{Basis: 5}, {Basis: 5}}, []int{5, 5}, true},
		{"grow one one", 21, 1, []FlexConstraint{{Grow: 1, Basis: 5}, {Grow: 1, Basis: 5}}, []int{10, 10}, true},
		{"grow one two", 20, 0, []FlexConstraint{{Grow: 1, Basis: 2}, {Grow: 2, Basis: 2}}, []int{7, 13}, true},
		{"max width caps growth", 20, 0, []FlexConstraint{{Grow: 1, Basis: 2, MaxWidth: 4}, {Grow: 1, Basis: 2}}, []int{4, 16}, true},
		{"shrink to minimum", 10, 1, []FlexConstraint{{Basis: 8, MinWidth: 4}, {Basis: 8, MinWidth: 4}}, []int{4, 5}, true},
		{"insufficient minimum", 7, 1, []FlexConstraint{{MinWidth: 4}, {MinWidth: 4}}, []int{4, 4}, false},
		{"deterministic rounding", 10, 0, []FlexConstraint{{Grow: 1, Basis: 1}, {Grow: 1, Basis: 1}, {Grow: 1, Basis: 1}}, []int{4, 3, 3}, true},
		{"one child", 8, 0, []FlexConstraint{{Grow: 1}}, []int{8}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, fits := AllocateRow(tt.available, tt.gap, tt.children)
			if !reflect.DeepEqual(got, tt.want) || fits != tt.fits {
				t.Fatalf("AllocateRow=%v,%v want %v,%v", got, fits, tt.want, tt.fits)
			}
		})
	}
}

func TestAllocateRowNeverReturnsNegativeOrZeroVisibleWidths(t *testing.T) {
	got, _ := AllocateRow(1, 3, []FlexConstraint{{}, {}, {}})
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	for i, w := range got {
		if w < 1 {
			t.Fatalf("width[%d]=%d", i, w)
		}
	}
}
