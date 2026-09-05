package protocol

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSchemaConnectsEveryComponentPropsAndWireShape(t *testing.T) {
	b, err := os.ReadFile("../assets/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	if _, ok := root["oneOf"]; !ok {
		t.Fatal("schema must describe legacyOperation/envelope/event as a top-level union")
	}
	text := string(b)
	for _, name := range []string{"boxProps", "textProps", "viewportProps", "tableProps", "inputProps", "actionsProps", "progressProps"} {
		ref := `"$ref": "#/$defs/` + name + `"`
		if strings.Count(text, ref) < 1 {
			t.Fatalf("%s is defined but not connected by $ref", name)
		}
	}
	for _, token := range []string{"hello", "hello_ack", "operation", "event", "telemetry"} {
		if !strings.Contains(text, `"`+token+`"`) {
			t.Fatalf("missing envelope kind %s", token)
		}
	}
}

func TestSchemaExposesEventCausalityFields(t *testing.T) {
	b, err := os.ReadFile("../assets/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, field := range []string{"related_seq", "through_seq", "revision"} {
		if !strings.Contains(text, `"`+field+`"`) {
			t.Fatalf("event schema missing %s", field)
		}
	}
}

func TestSchemaDiscriminatesEnvelopePayloadByKind(t *testing.T) {
	b, err := os.ReadFile("../assets/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatal(err)
	}
	defs := root["$defs"].(map[string]any)
	env := defs["envelope"].(map[string]any)
	variants, ok := env["oneOf"].([]any)
	if !ok || len(variants) != 5 {
		t.Fatalf("envelope must be five discriminated variants, got %#v", env["oneOf"])
	}
	text := string(b)
	for _, name := range []string{"helloEnvelope", "helloAckEnvelope", "operationEnvelope", "eventEnvelope", "telemetryEnvelope", "helloPayload", "helloAckPayload", "limits"} {
		if !strings.Contains(text, `"`+name+`"`) {
			t.Fatalf("schema missing %s", name)
		}
	}
}

func TestSchemaExposesV3InteractiveFields(t *testing.T) {
	b, err := os.ReadFile("../assets/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, field := range []string{"scrollable", "row_ids", "row_id"} {
		if !strings.Contains(text, `"`+field+`"`) {
			t.Fatalf("V3 schema missing %s", field)
		}
	}
}
