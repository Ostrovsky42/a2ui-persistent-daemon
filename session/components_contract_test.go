package session

import (
	"reflect"
	"testing"

	"a2ui/protocol"
)

func TestHelloAckComponentsAreExactV1EndpointAllowlist(t *testing.T) {
	s := New("s", protocol.DefaultLimits())
	ack, err := s.Negotiate(protocol.Hello{Versions: []int{protocol.Version}})
	if err != nil {
		t.Fatal(err)
	}
	want := protocol.AllNodeTypes()
	if !reflect.DeepEqual(ack.Components, want) {
		t.Fatalf("components=%v want=%v", ack.Components, want)
	}
	for _, component := range ack.Components {
		if !component.Valid() {
			t.Fatalf("advertised invalid component %q", component)
		}
	}
}
