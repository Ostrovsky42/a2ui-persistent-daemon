package document

import (
	"reflect"
	"testing"

	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func TestExistingUpsertRelocationUsesCreationIndexSemantics(t *testing.T) {
	t.Run("changed parent without index appends", func(t *testing.T) {
		d := New()
		d, _ = applyOK(t, d, up(1, "left", protocol.NodeBox, "root", `{}`))
		d, _ = applyOK(t, d, up(2, "right", protocol.NodeBox, "root", `{}`))
		d, _ = applyOK(t, d, up(3, "existing", protocol.NodeText, "right", `{}`))
		d, _ = applyOK(t, d, up(4, "moving", protocol.NodeText, "left", `{}`))

		next, eff, err := Apply(d, up(5, "moving", protocol.NodeText, "right", `{}`), protocol.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if got, want := next.Nodes["right"].Children, []string{"existing", "moving"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("children=%v want=%v", got, want)
		}
		if !eff.Changed {
			t.Fatal("relocation must change the document")
		}
	})

	t.Run("out of range index appends", func(t *testing.T) {
		d := New()
		d, _ = applyOK(t, d, up(1, "box", protocol.NodeBox, "root", `{}`))
		d, _ = applyOK(t, d, up(2, "a", protocol.NodeText, "box", `{}`))
		d, _ = applyOK(t, d, up(3, "b", protocol.NodeText, "box", `{}`))
		d, _ = applyOK(t, d, up(4, "c", protocol.NodeText, "box", `{}`))

		op := up(5, "a", protocol.NodeText, "", `{}`)
		op.Index = relocationIndex(99)
		next, _, err := Apply(d, op, protocol.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		if got, want := next.Nodes["box"].Children, []string{"b", "c", "a"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("children=%v want=%v", got, want)
		}
	})
}

func TestExistingUpsertRelocationResourceFailureRollsBack(t *testing.T) {
	limits := protocol.DefaultLimits()
	limits.MaxDepth = 2
	d := New()
	var err *protocol.Error
	d, _, err = Apply(d, up(1, "panel", protocol.NodeBox, "root", `{}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err = Apply(d, up(2, "leaf", protocol.NodeText, "panel", `{}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err = Apply(d, up(3, "target", protocol.NodeBox, "root", `{}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	d, _, err = Apply(d, up(4, "inner", protocol.NodeBox, "target", `{}`), limits)
	if err != nil {
		t.Fatal(err)
	}
	before := d.Clone()

	next, _, err := Apply(d, up(5, "panel", protocol.NodeBox, "inner", `{}`), limits)
	if err == nil || err.Code != "resource.depth_limit" {
		t.Fatalf("got %#v want resource.depth_limit", err)
	}
	if !reflect.DeepEqual(next, before) || !reflect.DeepEqual(d, before) {
		t.Fatal("failed relocation changed authoritative document")
	}
}
