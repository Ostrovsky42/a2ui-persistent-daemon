package document

import (
	"reflect"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func relocationIndex(n int) *int { return &n }

func TestExistingUpsertReordersWithinParent(t *testing.T) {
	d := New()
	d, _ = applyOK(t, d, up(1, "box", protocol.NodeBox, "root", `{}`))
	d, _ = applyOK(t, d, up(2, "a", protocol.NodeText, "box", `{}`))
	d, _ = applyOK(t, d, up(3, "b", protocol.NodeText, "box", `{}`))
	d, _ = applyOK(t, d, up(4, "c", protocol.NodeText, "box", `{}`))

	op := up(5, "b", protocol.NodeText, "", `{}`)
	op.Index = relocationIndex(0)
	next, eff, err := Apply(d, op, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := next.Nodes["box"].Children, []string{"b", "a", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("children=%v want=%v", got, want)
	}
	if next.Nodes["b"].Parent != "box" {
		t.Fatalf("parent=%q want=box", next.Nodes["b"].Parent)
	}
	if !eff.Changed || len(eff.Removed) != 0 || eff.TypeChanged {
		t.Fatalf("effect=%+v", eff)
	}
}

func TestExistingUpsertRelocatesSubtreeAcrossParents(t *testing.T) {
	d := New()
	d, _ = applyOK(t, d, up(1, "left", protocol.NodeBox, "root", `{}`))
	d, _ = applyOK(t, d, up(2, "right", protocol.NodeBox, "root", `{}`))
	d, _ = applyOK(t, d, up(3, "panel", protocol.NodeBox, "left", `{}`))
	d, _ = applyOK(t, d, up(4, "input", protocol.NodeInput, "panel", `{}`))

	op := up(5, "panel", protocol.NodeBox, "right", `{}`)
	op.Index = relocationIndex(0)
	next, eff, err := Apply(d, op, protocol.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if got := next.Nodes["left"].Children; len(got) != 0 {
		t.Fatalf("left children=%v want=[]", got)
	}
	if got, want := next.Nodes["right"].Children, []string{"panel"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("right children=%v want=%v", got, want)
	}
	if next.Nodes["panel"].Parent != "right" {
		t.Fatalf("panel parent=%q want=right", next.Nodes["panel"].Parent)
	}
	if next.Nodes["input"].Parent != "panel" {
		t.Fatalf("descendant parent changed: %q", next.Nodes["input"].Parent)
	}
	if !eff.Changed || len(eff.Removed) != 0 || eff.TypeChanged {
		t.Fatalf("effect=%+v", eff)
	}
	if inv := Validate(next, protocol.DefaultLimits()); inv != nil {
		t.Fatalf("relocated document invalid: %v", inv)
	}
}

func TestExistingUpsertRejectsCycleTransactionally(t *testing.T) {
	d := New()
	d, _ = applyOK(t, d, up(1, "parent", protocol.NodeBox, "root", `{}`))
	d, _ = applyOK(t, d, up(2, "child", protocol.NodeBox, "parent", `{}`))
	before := d.Clone()

	_, _, err := Apply(d, up(3, "parent", protocol.NodeBox, "child", `{}`), protocol.DefaultLimits())
	if err == nil || err.Code != "document.cycle" {
		t.Fatalf("got %#v", err)
	}
	if !reflect.DeepEqual(d, before) {
		t.Fatal("cycle rejection mutated authoritative document")
	}
}

func TestExistingUpsertValidatesRelocationTarget(t *testing.T) {
	d := New()
	d, _ = applyOK(t, d, up(1, "panel", protocol.NodeBox, "root", `{}`))
	d, _ = applyOK(t, d, up(2, "leaf", protocol.NodeText, "root", `{}`))

	for _, tc := range []struct {
		name   string
		parent string
		code   string
	}{
		{name: "missing", parent: "missing", code: "document.parent_not_found"},
		{name: "leaf", parent: "leaf", code: "document.parent_not_container"},
		{name: "self", parent: "panel", code: "document.cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := d.Clone()
			_, _, err := Apply(d, up(10, "panel", protocol.NodeBox, tc.parent, `{}`), protocol.DefaultLimits())
			if err == nil || err.Code != tc.code {
				t.Fatalf("got %#v want code=%s", err, tc.code)
			}
			if !reflect.DeepEqual(d, before) {
				t.Fatal("failed relocation mutated authoritative document")
			}
		})
	}
}
