package protocol

import (
	"encoding/json"
	"testing"
)

func TestValidateOperationRejectsUnknownProperty(t *testing.T) {
	op := Operation{V: 1, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"text":"ok","wat":1}`)}
	if err := ValidateOperation(op, DefaultLimits()); err == nil || err.Code != "schema.unknown_property" {
		t.Fatalf("expected schema.unknown_property, got %#v", err)
	}
}

func TestValidateOperationRejectsInvalidColor(t *testing.T) {
	op := Operation{V: 1, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"style":{"fg":"#fff"}}`)}
	if err := ValidateOperation(op, DefaultLimits()); err == nil || err.Code != "schema.invalid_color" {
		t.Fatalf("expected schema.invalid_color, got %#v", err)
	}
}

func TestInputForceIsPolicyNotPersistentProp(t *testing.T) {
	normalized, policy, err := NormalizeProps(NodeInput, json.RawMessage(`{"placeholder":"p","value":"x","force":true}`), true)
	if err != nil {
		t.Fatal(err)
	}
	if !policy.ForceInputValue {
		t.Fatal("force policy not extracted")
	}
	if _, ok := normalized["force"]; ok {
		t.Fatal("force must not be retained in normalized props")
	}
}

func TestFullNormalizeAppliesDefaults(t *testing.T) {
	normalized, _, err := NormalizeProps(NodeBox, json.RawMessage(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	var dir string
	if err := json.Unmarshal(normalized["dir"], &dir); err != nil {
		t.Fatal(err)
	}
	if dir != "col" {
		t.Fatalf("expected col default, got %q", dir)
	}
}

func TestLimitsAreFinite(t *testing.T) {
	l := DefaultLimits()
	if l.MaxMessageBytes <= 0 || l.MaxNodes <= 0 || l.MaxDepth <= 0 || l.MaxPendingEvents <= 0 {
		t.Fatalf("limits must be finite: %+v", l)
	}
}

func TestNestedTableAndActionPropsRejectUnknownFields(t *testing.T) {
	cases := []Operation{
		{V: 1, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"columns":[{"title":"A","width":3,"wat":1}]}`)},
		{V: 1, Op: OpUpsert, ID: "a", Type: NodeActions, Props: json.RawMessage(`{"items":[{"key":"r","label":"R","action":"r","wat":1}]}`)},
	}
	for _, op := range cases {
		if err := ValidateOperation(op, DefaultLimits()); err == nil || err.Code != "schema.unknown_property" {
			t.Fatalf("op=%s got %#v", op.Type, err)
		}
	}
}

func TestColumnWidthWhenPresentMustBePositive(t *testing.T) {
	op := Operation{V: 1, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"columns":[{"title":"A","width":0}]}`)}
	if err := ValidateOperation(op, DefaultLimits()); err == nil || err.Code != "schema.invalid_props" {
		t.Fatalf("got %#v", err)
	}
}

func TestPropsOperationHonorsMessageLimit(t *testing.T) {
	l := DefaultLimits()
	l.MaxMessageBytes = 8
	op := Operation{V: 1, Op: OpProps, ID: "x", Props: json.RawMessage(`{"text":"0123456789"}`)}
	if err := ValidateOperation(op, l); err == nil || err.Code != "resource.message_too_large" {
		t.Fatalf("got %#v", err)
	}
}

func TestValidateOperationRejectsNegativeSequence(t *testing.T) {
	op := Operation{V: Version, Seq: -1, Op: OpCommit}
	err := ValidateOperation(op, DefaultLimits())
	if err == nil || err.Code != "protocol.invalid_sequence" {
		t.Fatalf("got %#v", err)
	}
}

func TestPropsOperationRejectsExplicitNullObject(t *testing.T) {
	op := Operation{V: Version, Seq: 1, Op: OpProps, ID: "x", Props: json.RawMessage(`null`)}
	err := ValidateOperation(op, DefaultLimits())
	if err == nil || err.Code != "schema.invalid_props" {
		t.Fatalf("expected schema.invalid_props, got %#v", err)
	}
}

func TestNormalizePropsRejectsExplicitNullWhenProvided(t *testing.T) {
	_, _, err := NormalizeProps(NodeText, json.RawMessage(`null`), true)
	if err == nil || err.Code != "schema.invalid_props" {
		t.Fatalf("expected schema.invalid_props, got %#v", err)
	}
}

func TestValidateLimitsRejectsNonPositiveFields(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxPendingEvents = 0
	err := ValidateLimits(limits)
	if err == nil || err.Code != "schema.invalid_limits" {
		t.Fatalf("expected schema.invalid_limits, got %#v", err)
	}
}

func TestHelloValidationMatchesSchemaUniquenessRules(t *testing.T) {
	cases := []Hello{
		{Versions: []int{1, 1}},
		{Versions: []int{1}, Features: []string{"x", "x"}},
		{Versions: []int{1}, Features: []string{""}},
	}
	for _, hello := range cases {
		if err := ValidateHello(hello); err == nil || err.Code != "schema.invalid_hello" {
			t.Fatalf("hello=%+v got %#v", hello, err)
		}
	}
}

func TestHelloAckValidationMatchesSchemaAndRequiresFiniteLimits(t *testing.T) {
	ack := HelloAck{Version: Version, Features: []string{"x", "x"}, Components: []NodeType{NodeBox}, Limits: DefaultLimits()}
	if err := ValidateHelloAck(ack); err == nil || err.Code != "schema.invalid_hello_ack" {
		t.Fatalf("duplicate features got %#v", err)
	}
	ack = HelloAck{Version: Version, Features: []string{}, Components: []NodeType{NodeBox, NodeBox}, Limits: DefaultLimits()}
	if err := ValidateHelloAck(ack); err == nil || err.Code != "schema.invalid_hello_ack" {
		t.Fatalf("duplicate components got %#v", err)
	}
	ack = HelloAck{Version: Version, Limits: Limits{}}
	if err := ValidateHelloAck(ack); err == nil || err.Code != "schema.invalid_limits" {
		t.Fatalf("zero limits got %#v", err)
	}
}

func TestEffectiveLimitsFillsOnlyMissingValues(t *testing.T) {
	limits := Limits{MaxNodes: 7}
	effective := EffectiveLimits(limits)
	if effective.MaxNodes != 7 {
		t.Fatalf("explicit max_nodes overwritten: %+v", effective)
	}
	if err := ValidateLimits(effective); err != nil {
		t.Fatalf("effective limits are not finite: %v", err)
	}
}

func TestDefaultLimitsIncludeAggregateDocumentBudget(t *testing.T) {
	limits := DefaultLimits()
	if limits.MaxDocumentBytes < limits.MaxTotalTextBytes {
		t.Fatalf("document budget %d is smaller than text budget %d", limits.MaxDocumentBytes, limits.MaxTotalTextBytes)
	}
	if err := ValidateLimits(limits); err != nil {
		t.Fatal(err)
	}
}

func TestPresentationVariantsAndFlexValidateStrictly(t *testing.T) {
	valid := []Operation{
		{V: 1, Seq: 1, Op: OpUpsert, ID: "box", Type: NodeBox, Props: json.RawMessage(`{"variant":"card","align":"center","responsive":"stack","flex":{"grow":2,"basis":30,"min_width":20,"max_width":80}}`)},
		{V: 1, Seq: 2, Op: OpUpsert, ID: "text", Type: NodeText, Props: json.RawMessage(`{"variant":"title","flex":{"grow":1}}`)},
		{V: 1, Seq: 3, Op: OpUpsert, ID: "actions", Type: NodeActions, Props: json.RawMessage(`{"variant":"toolbar","items":[]}`)},
		{V: 1, Seq: 4, Op: OpUpsert, ID: "progress", Type: NodeProgress, Props: json.RawMessage(`{"variant":"spinner","state":"loading","value":0.5}`)},
		{V: 1, Seq: 5, Op: OpUpsert, ID: "table", Type: NodeTable, Props: json.RawMessage(`{"variant":"dense","columns":[],"rows":[]}`)},
		{V: 1, Seq: 6, Op: OpUpsert, ID: "vp", Type: NodeViewport, Props: json.RawMessage(`{"height":5,"wrap":true,"follow_tail":true,"flex":{"min_width":3}}`)},
	}
	for _, op := range valid {
		if err := ValidateOperation(op, DefaultLimits()); err != nil {
			t.Fatalf("valid %s rejected: %v", op.Type, err)
		}
	}

	invalid := []Operation{
		{V: 1, Seq: 10, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"variant":"hero"}`)},
		{V: 1, Seq: 11, Op: OpUpsert, ID: "x", Type: NodeBox, Props: json.RawMessage(`{"variant":"modal"}`)},
		{V: 1, Seq: 12, Op: OpUpsert, ID: "x", Type: NodeActions, Props: json.RawMessage(`{"variant":"grid"}`)},
		{V: 1, Seq: 13, Op: OpUpsert, ID: "x", Type: NodeProgress, Props: json.RawMessage(`{"variant":"ring"}`)},
		{V: 1, Seq: 14, Op: OpUpsert, ID: "x", Type: NodeProgress, Props: json.RawMessage(`{"state":"paused"}`)},
		{V: 1, Seq: 15, Op: OpUpsert, ID: "x", Type: NodeTable, Props: json.RawMessage(`{"variant":"huge"}`)},
		{V: 1, Seq: 16, Op: OpUpsert, ID: "x", Type: NodeBox, Props: json.RawMessage(`{"align":"middle"}`)},
		{V: 1, Seq: 17, Op: OpUpsert, ID: "x", Type: NodeBox, Props: json.RawMessage(`{"responsive":"wrap"}`)},
		{V: 1, Seq: 18, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"flex":{"grow":-1}}`)},
		{V: 1, Seq: 19, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"flex":{"basis":-1}}`)},
		{V: 1, Seq: 20, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"flex":{"min_width":10,"max_width":5}}`)},
		{V: 1, Seq: 21, Op: OpUpsert, ID: "x", Type: NodeText, Props: json.RawMessage(`{"flex":{"wat":1}}`)},
	}
	for _, op := range invalid {
		if err := ValidateOperation(op, DefaultLimits()); err == nil {
			t.Fatalf("invalid %s props accepted: %s", op.Type, op.Props)
		}
	}
}

func TestPresentationDefaultsAreBackwardCompatible(t *testing.T) {
	box, _, err := NormalizeProps(NodeBox, json.RawMessage(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	assertRawString := func(props map[string]json.RawMessage, key, want string) {
		t.Helper()
		var got string
		if e := json.Unmarshal(props[key], &got); e != nil {
			t.Fatalf("%s: %v", key, e)
		}
		if got != want {
			t.Fatalf("%s=%q want %q", key, got, want)
		}
	}
	assertRawString(box, "variant", "plain")
	assertRawString(box, "align", "start")
	assertRawString(box, "responsive", "none")

	text, _, _ := NormalizeProps(NodeText, json.RawMessage(`{}`), true)
	assertRawString(text, "variant", "body")
	actions, _, _ := NormalizeProps(NodeActions, json.RawMessage(`{}`), true)
	assertRawString(actions, "variant", "inline")
	progress, _, _ := NormalizeProps(NodeProgress, json.RawMessage(`{}`), true)
	assertRawString(progress, "variant", "bar")
	assertRawString(progress, "state", "normal")
	table, _, _ := NormalizeProps(NodeTable, json.RawMessage(`{}`), true)
	assertRawString(table, "variant", "normal")
	vp, _, _ := NormalizeProps(NodeViewport, json.RawMessage(`{}`), true)
	var tail bool
	if e := json.Unmarshal(vp["follow_tail"], &tail); e != nil || tail {
		t.Fatalf("follow_tail=%v err=%v", tail, e)
	}
}

func TestV3InteractivePropsValidateStrictly(t *testing.T) {
	valid := []Operation{
		{V: 1, Seq: 30, Op: OpUpsert, ID: "vp", Type: NodeViewport, Props: json.RawMessage(`{"scrollable":true}`)},
		{V: 1, Seq: 31, Op: OpUpsert, ID: "tbl", Type: NodeTable, Props: json.RawMessage(`{"selectable":true,"action":"open_job","columns":[{"title":"Job","width":10}],"rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)},
	}
	for _, op := range valid {
		if err := ValidateOperation(op, DefaultLimits()); err != nil {
			t.Fatalf("valid %s rejected: %#v", op.Type, err)
		}
	}

	invalid := []struct {
		name string
		op   Operation
	}{
		{"viewport scrollable type", Operation{V: 1, Seq: 40, Op: OpUpsert, ID: "vp", Type: NodeViewport, Props: json.RawMessage(`{"scrollable":"yes"}`)}},
		{"table action type", Operation{V: 1, Seq: 41, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"action":1}`)}},
		{"row ids type", Operation{V: 1, Seq: 42, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"row_ids":"x"}`)}},
		{"row id element type", Operation{V: 1, Seq: 43, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"rows":[["A"]],"row_ids":[1]}`)}},
		{"empty row id", Operation{V: 1, Seq: 44, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"rows":[["A"]],"row_ids":[""]}`)}},
		{"duplicate row id", Operation{V: 1, Seq: 45, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"rows":[["A"],["B"]],"row_ids":["same","same"]}`)}},
		{"row ids length", Operation{V: 1, Seq: 46, Op: OpUpsert, ID: "t", Type: NodeTable, Props: json.RawMessage(`{"rows":[["A"],["B"]],"row_ids":["a"]}`)}},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateOperation(tc.op, DefaultLimits()); err == nil {
				t.Fatalf("invalid props accepted: %s", tc.op.Props)
			}
		})
	}
}

func TestV3PresentationDefaultsRemainPassive(t *testing.T) {
	vp, _, err := NormalizeProps(NodeViewport, json.RawMessage(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	var scrollable bool
	if err := json.Unmarshal(vp["scrollable"], &scrollable); err != nil || scrollable {
		t.Fatalf("scrollable=%v err=%v", scrollable, err)
	}

	table, _, err := NormalizeProps(NodeTable, json.RawMessage(`{}`), true)
	if err != nil {
		t.Fatal(err)
	}
	var action string
	if err := json.Unmarshal(table["action"], &action); err != nil || action != "" {
		t.Fatalf("action=%q err=%v", action, err)
	}
	if _, ok := table["row_ids"]; ok {
		t.Fatal("row_ids must remain absent unless explicitly supplied")
	}
}
