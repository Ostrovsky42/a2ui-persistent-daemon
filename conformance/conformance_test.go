package conformance

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"testing"

	"a2ui/engine"
	"a2ui/protocol"
	"a2ui/wire"
)

type summary struct {
	Revision    uint64            `json:"revision"`
	Focused     string            `json:"focused"`
	NodeTypes   map[string]string `json:"node_types"`
	MainGap     int               `json:"main_gap"`
	TitleStream string            `json:"title_stream"`
	Commit      commitSummary     `json:"commit"`
}
type commitSummary struct {
	Ev         string `json:"ev"`
	Revision   uint64 `json:"revision"`
	ThroughSeq uint64 `json:"through_seq"`
	Frame      string `json:"frame"`
}

func TestBasicConformanceReplay(t *testing.T) {
	f, err := os.Open("testdata/basic.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	e := engine.New(protocol.DefaultLimits(), 32, engine.NewNoopActions())
	rd := wire.NewNDJSONReader(f, protocol.DefaultLimits().MaxMessageBytes)
	for {
		env, perr, err := rd.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if perr != nil {
			t.Fatal(perr)
		}
		var op protocol.Operation
		if err := json.Unmarshal(env.Payload, &op); err != nil {
			t.Fatal(err)
		}
		if perr := e.Apply(op); perr != nil {
			t.Fatal(perr)
		}
	}
	d := e.Document()
	got := summary{Revision: d.Revision, Focused: e.FocusedID(), NodeTypes: map[string]string{}, TitleStream: d.Nodes["title"].Text}
	for id, n := range d.Nodes {
		got.NodeTypes[id] = string(n.Type)
	}
	_ = json.Unmarshal(d.Nodes["main"].Props["gap"], &got.MainGap)
	if perr := e.Publish(); perr != nil {
		t.Fatal(perr)
	}
	ev, ok := e.NextEvent()
	if !ok {
		t.Fatal("missing commit event")
	}
	got.Commit = commitSummary{Ev: ev.Ev, Revision: ev.Revision, ThroughSeq: ev.ThroughSeq, Frame: ev.Frame}
	wantBytes, err := os.ReadFile("testdata/basic.want.json")
	if err != nil {
		t.Fatal(err)
	}
	var want summary
	if err := json.Unmarshal(wantBytes, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		gb, _ := json.MarshalIndent(got, "", "  ")
		wb, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("got\n%s\nwant\n%s", gb, wb)
	}
}
