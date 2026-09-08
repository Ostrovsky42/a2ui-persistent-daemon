package ipc

import (
	"fmt"
	"testing"
)

func TestDecodeSemanticActionInvokeInteraction(t *testing.T) {
	raw := []byte(fmt.Sprintf(`{"v":%d,"kind":"interaction","interaction":{"type":"action_invoke","id":"actions","action":"approve","args":{"target":"prod"}}}`, Version))
	msg, perr := DecodeMessage(raw)
	if perr != nil {
		t.Fatalf("semantic action invocation must decode without a physical key: %v", perr)
	}
	if msg.Interaction == nil || msg.Interaction.Type != InteractionType("action_invoke") {
		t.Fatalf("interaction=%+v", msg.Interaction)
	}
}

func TestDecodeSemanticTableSelectByRowIDInteraction(t *testing.T) {
	raw := []byte(fmt.Sprintf(`{"v":%d,"kind":"interaction","interaction":{"type":"table_select","id":"jobs","row_id":"job-42"}}`, Version))
	msg, perr := DecodeMessage(raw)
	if perr != nil {
		t.Fatalf("semantic table selection must decode by stable row identity: %v", perr)
	}
	if msg.Interaction == nil || msg.Interaction.Type != InteractionType("table_select") {
		t.Fatalf("interaction=%+v", msg.Interaction)
	}
}
