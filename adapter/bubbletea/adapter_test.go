package bubbletea

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Ostrovsky42/agent-interaction-runtime/engine"
	"github.com/Ostrovsky42/agent-interaction-runtime/protocol"
	a2runtime "github.com/Ostrovsky42/agent-interaction-runtime/runtime"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newTestEngine() *engine.Engine {
	limits := protocol.DefaultLimits()
	return engine.New(limits, limits.MaxPendingEvents, engine.NewNoopActions())
}

func TestBubbleTeaModelLifecycle(t *testing.T) {
	eng := newTestEngine()
	model := NewModel(eng, DefaultTheme, nil)

	// Init command
	cmd := model.Init()
	if cmd != nil {
		t.Fatal("expected nil cmd for nil updateChan")
	}

	// Window resize
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m := updated.(Model)
	if m.Width != 100 || m.Height != 30 {
		t.Fatalf("expected width 100, height 30; got %d, %d", m.Width, m.Height)
	}

	// Quit
	updated2, quitCmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m2 := updated2.(Model)
	if !m2.Quitting || quitCmd == nil {
		t.Fatal("expected model to quit on ctrl+c")
	}
	if !strings.Contains(m2.View(), "Session terminated") {
		t.Fatalf("expected quit message in view, got %q", m2.View())
	}
}

func TestBubbleTeaInputEditingAndSubmit(t *testing.T) {
	eng := newTestEngine()

	// Add input node
	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "name-input",
		Type:   protocol.NodeInput,
		Parent: "root",
		Props:  json.RawMessage(`{"placeholder":"Your name...","value":"","action":"save_name"}`),
	})
	_ = eng.Focus("name-input")

	model := NewModel(eng, DefaultTheme, nil)

	// Type "Bob"
	for _, ch := range "Bob" {
		updated, _ := model.Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{ch},
		})
		model = updated.(Model)
	}

	if val := eng.InputValue("name-input"); val != "Bob" {
		t.Fatalf("expected input value 'Bob', got %q", val)
	}

	// Backspace
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	model = updated.(Model)
	if val := eng.InputValue("name-input"); val != "Bo" {
		t.Fatalf("expected input value 'Bo', got %q", val)
	}

	// Enter / Submit
	_, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	ev, ok := eng.NextEvent()
	if !ok {
		t.Fatal("expected submit event on engine broker")
	}
	if ev.Ev != "submit" || ev.ID != "name-input" || ev.Value != "Bo" || ev.Action != "save_name" {
		t.Fatalf("unexpected submit event: %+v", ev)
	}
}

func TestBubbleTeaTabFocusCycling(t *testing.T) {
	eng := newTestEngine()

	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "in1",
		Type:   protocol.NodeInput,
		Parent: "root",
		Props:  json.RawMessage(`{"value":"1"}`),
	})
	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    2,
		Op:     protocol.OpUpsert,
		ID:     "in2",
		Type:   protocol.NodeInput,
		Parent: "root",
		Props:  json.RawMessage(`{"value":"2"}`),
	})
	_ = eng.Focus("in1")

	model := NewModel(eng, DefaultTheme, nil)

	// Tab -> in2
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if eng.FocusedID() != "in2" {
		t.Fatalf("expected focus on in2, got %q", eng.FocusedID())
	}

	// Tab -> in1 (wrap)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	if eng.FocusedID() != "in1" {
		t.Fatalf("expected focus on in1, got %q", eng.FocusedID())
	}

	// Shift+Tab -> in2
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	model = updated.(Model)
	if eng.FocusedID() != "in2" {
		t.Fatalf("expected focus on in2 after shift+tab, got %q", eng.FocusedID())
	}
}

func TestBubbleTeaActionHotkeyDispatch(t *testing.T) {
	actions := a2runtime.NewActionRegistry(2*time.Second, 4)
	actionTriggered := false
	_ = actions.Register("toggle_dark", func(ctx context.Context, args json.RawMessage) error {
		actionTriggered = true
		return nil
	})

	limits := protocol.DefaultLimits()
	eng := engine.New(limits, limits.MaxPendingEvents, actions)

	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "act-box",
		Type:   protocol.NodeActions,
		Parent: "root",
		Props:  json.RawMessage(`{"items":[{"key":"d","label":"Dark Mode","action":"toggle_dark"}]}`),
	})

	model := NewModel(eng, DefaultTheme, nil)

	// Press 'd'
	_, _ = model.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune{'d'},
	})

	if !actionTriggered {
		t.Fatal("expected action handler for 'd' to execute")
	}

	ev, ok := eng.NextEvent()
	if !ok || ev.Ev != "action_result" || ev.Action != "toggle_dark" {
		t.Fatalf("expected action_result event, got %+v", ev)
	}
}

func TestBubbleTeaCommitBarrierOnView(t *testing.T) {
	eng := newTestEngine()

	// Apply node and commit
	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "heading",
		Type:   protocol.NodeText,
		Parent: "root",
		Props:  json.RawMessage(`{"text":"Dashboard v2"}`),
	})
	_ = eng.Apply(protocol.Operation{
		V:     1,
		Seq:   2,
		Op:    protocol.OpCommit,
		Frame: "frame-dash-01",
	})

	// Before View(): NeedsPublish must be true, and broker must NOT have emitted committed event
	if !eng.NeedsPublish() {
		t.Fatal("expected NeedsPublish to be true before View()")
	}
	if _, ok := eng.NextEvent(); ok {
		t.Fatal("broker prematurely emitted event before View() commit barrier!")
	}

	model := NewModel(eng, DefaultTheme, nil)

	// Call View() — this triggers the renderer publication barrier
	viewStr := model.View()
	if !strings.Contains(viewStr, "Dashboard v2") {
		t.Fatalf("view missing heading text, got:\n%s", viewStr)
	}

	// After View(): NeedsPublish must be false
	if eng.NeedsPublish() {
		t.Fatal("expected NeedsPublish to be false after View()")
	}

	// Broker now releases the committed event!
	ev, ok := eng.NextEvent()
	if !ok {
		t.Fatal("expected committed event after View() publication barrier")
	}
	if ev.Ev != "committed" || ev.Frame != "frame-dash-01" || ev.ThroughSeq != 2 {
		t.Fatalf("unexpected committed event: %+v", ev)
	}

	if model.PublishCount() != 1 {
		t.Fatalf("expected PublishCount 1, got %d", model.PublishCount())
	}
}

func TestBubbleTeaTableNavigation(t *testing.T) {
	eng := newTestEngine()

	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "tbl",
		Type:   protocol.NodeTable,
		Parent: "root",
		Props: json.RawMessage(`{
			"selectable": true,
			"columns": [{"title":"Col1","width":10}],
			"rows": [["Row0"],["Row1"],["Row2"]]
		}`),
	})
	_ = eng.Focus("tbl")

	model := NewModel(eng, DefaultTheme, nil)

	// KeyDown -> row 1
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if sel, ok := eng.SelectedTableRow("tbl"); !ok || sel.Index != 1 {
		t.Fatalf("expected row 1, got %+v ok=%v", sel, ok)
	}

	// KeyDown -> row 2
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if sel, ok := eng.SelectedTableRow("tbl"); !ok || sel.Index != 2 {
		t.Fatalf("expected row 2, got %+v ok=%v", sel, ok)
	}

	// KeyDown clamped at 2
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	if sel, ok := eng.SelectedTableRow("tbl"); !ok || sel.Index != 2 {
		t.Fatalf("expected row 2 clamped, got %+v ok=%v", sel, ok)
	}

	// KeyUp -> row 1
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	if sel, ok := eng.SelectedTableRow("tbl"); !ok || sel.Index != 1 {
		t.Fatalf("expected row 1, got %+v ok=%v", sel, ok)
	}
}

func TestBubbleTeaRowLayoutWithGaps(t *testing.T) {
	eng := newTestEngine()

	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "row-box",
		Type:   protocol.NodeBox,
		Parent: "root",
		Props:  json.RawMessage(`{"dir":"row","gap":2,"padding":1,"border":"normal"}`),
	})
	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    2,
		Op:     protocol.OpUpsert,
		ID:     "col1",
		Type:   protocol.NodeText,
		Parent: "row-box",
		Props:  json.RawMessage(`{"text":"Left Pane"}`),
	})
	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    3,
		Op:     protocol.OpUpsert,
		ID:     "col2",
		Type:   protocol.NodeText,
		Parent: "row-box",
		Props:  json.RawMessage(`{"text":"Right Pane"}`),
	})

	model := NewModel(eng, DefaultTheme, nil)
	view := model.View()
	if !strings.Contains(view, "Left Pane") || !strings.Contains(view, "Right Pane") {
		t.Fatalf("row layout missing child columns, got:\n%s", view)
	}
}

func TestBubbleTeaCombinedTextPropsAndStreaming(t *testing.T) {
	eng := newTestEngine()

	// Initial text in props
	_ = eng.Apply(protocol.Operation{
		V:      1,
		Seq:    1,
		Op:     protocol.OpUpsert,
		ID:     "txt",
		Type:   protocol.NodeText,
		Parent: "root",
		Props:  json.RawMessage(`{"text":"Initial: "}`),
	})

	// Streamed text appended via OpText
	_ = eng.Apply(protocol.Operation{
		V:    1,
		Seq:  2,
		Op:   protocol.OpText,
		ID:   "txt",
		Text: "Streamed Chunk",
	})

	model := NewModel(eng, DefaultTheme, nil)
	view := model.View()
	if !strings.Contains(view, "Initial: Streamed Chunk") {
		t.Fatalf("expected combined initial and streamed text, got:\n%s", view)
	}
}

func TestRendererPresetsAreDistinctForSameDocument(t *testing.T) {
	eng := newTestEngine()
	ops := []protocol.Operation{
		{V: 1, Seq: 100, Op: protocol.OpUpsert, ID: "card", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{"variant":"card","dir":"col","gap":1}`)},
		{V: 1, Seq: 101, Op: protocol.OpUpsert, ID: "title2", Type: protocol.NodeText, Parent: "card", Props: json.RawMessage(`{"text":"Renderer V2","variant":"title"}`)},
		{V: 1, Seq: 102, Op: protocol.OpUpsert, ID: "ok", Type: protocol.NodeProgress, Parent: "card", Props: json.RawMessage(`{"variant":"compact","state":"success","value":1,"label":"Ready"}`)},
		{V: 1, Seq: 103, Op: protocol.OpUpsert, ID: "acts2", Type: protocol.NodeActions, Parent: "card", Props: json.RawMessage(`{"variant":"toolbar","items":[{"key":"r","label":"Run","action":"run"}]}`)},
	}
	for _, op := range ops {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	doc := eng.Document()
	inputs := eng.InputValues()
	var frames []string
	for _, preset := range []Preset{PresetMinimal, PresetDashboard, PresetDense} {
		r := NewRendererWithPreset(DefaultTheme, preset)
		frames = append(frames, r.RenderTree(doc, "", inputs, map[string]int{}, 70, 20))
	}
	if frames[0] == frames[1] || frames[1] == frames[2] || frames[0] == frames[2] {
		t.Fatalf("presets must produce distinct frames")
	}
	for _, frame := range frames {
		for _, text := range []string{"Renderer V2", "Ready", "Run"} {
			if !strings.Contains(frame, text) {
				t.Fatalf("frame lost semantic content %q:\n%s", text, frame)
			}
		}
	}
}

func TestRendererResponsiveRowStacksWhenMinimumsDoNotFit(t *testing.T) {
	eng := newTestEngine()
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 110, Op: protocol.OpUpsert, ID: "responsive", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{"dir":"row","responsive":"stack","gap":1}`)},
		{V: 1, Seq: 111, Op: protocol.OpUpsert, ID: "left", Type: protocol.NodeText, Parent: "responsive", Props: json.RawMessage(`{"text":"LEFT","flex":{"grow":1,"min_width":24}}`)},
		{V: 1, Seq: 112, Op: protocol.OpUpsert, ID: "right", Type: protocol.NodeText, Parent: "responsive", Props: json.RawMessage(`{"text":"RIGHT","flex":{"grow":1,"min_width":24}}`)},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	wide := r.RenderTree(eng.Document(), "", eng.InputValues(), map[string]int{}, 70, 10)
	narrow := r.RenderTree(eng.Document(), "", eng.InputValues(), map[string]int{}, 40, 10)
	if !lineContainsBoth(wide, "LEFT", "RIGHT") {
		t.Fatalf("wide row did not remain horizontal:\n%s", wide)
	}
	if lineContainsBoth(narrow, "LEFT", "RIGHT") {
		t.Fatalf("narrow row did not stack:\n%s", narrow)
	}
}

func TestViewportClipsAndFollowsTail(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 120, Op: protocol.OpUpsert, ID: "vp2", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"height":2,"wrap":true,"follow_tail":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 121, Op: protocol.OpUpsert, ID: "log", Type: protocol.NodeText, Parent: "vp2", Props: json.RawMessage(`{"text":"one\ntwo\nthree\nfour"}`)}); err != nil {
		t.Fatal(err)
	}
	r := NewRendererWithPreset(DefaultTheme, PresetMinimal)
	tail := r.RenderTree(eng.Document(), "", eng.InputValues(), map[string]int{}, 40, 10)
	if !strings.Contains(tail, "three") || !strings.Contains(tail, "four") || strings.Contains(tail, "one") {
		t.Fatalf("bad tail viewport:\n%s", tail)
	}
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 122, Op: protocol.OpProps, ID: "vp2", Props: json.RawMessage(`{"follow_tail":false}`)}); err != nil {
		t.Fatal(err)
	}
	head := r.RenderTree(eng.Document(), "", eng.InputValues(), map[string]int{}, 40, 10)
	if !strings.Contains(head, "one") || !strings.Contains(head, "two") || strings.Contains(head, "four") {
		t.Fatalf("bad head viewport:\n%s", head)
	}
}

func TestSpinnerPhaseIsDeterministicAndRendererLocal(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 130, Op: protocol.OpUpsert, ID: "spin", Type: protocol.NodeProgress, Parent: "root", Props: json.RawMessage(`{"variant":"spinner","state":"loading","label":"Connecting"}`)}); err != nil {
		t.Fatal(err)
	}
	r := NewRendererWithPreset(DefaultTheme, PresetDashboard)
	doc := eng.Document()
	one := r.RenderTreeWithState(doc, "", eng.InputValues(), map[string]int{}, 40, 10, RenderState{AnimationPhase: 0, CursorVisible: true})
	two := r.RenderTreeWithState(doc, "", eng.InputValues(), map[string]int{}, 40, 10, RenderState{AnimationPhase: 1, CursorVisible: true})
	again := r.RenderTreeWithState(doc, "", eng.InputValues(), map[string]int{}, 40, 10, RenderState{AnimationPhase: 0, CursorVisible: true})
	if one == two {
		t.Fatal("spinner phase did not change frame")
	}
	if one != again {
		t.Fatal("same animation phase must render deterministically")
	}
	if eng.Document().Revision != doc.Revision {
		t.Fatal("rendering changed document revision")
	}
}

func TestAnimationTickDoesNotPublishOrAdvanceDocument(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 140, Op: protocol.OpUpsert, ID: "spin2", Type: protocol.NodeProgress, Parent: "root", Props: json.RawMessage(`{"variant":"spinner","state":"loading","label":"Loading"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 141, Op: protocol.OpCommit, Frame: "initial"}); err != nil {
		t.Fatal(err)
	}
	m := NewModelWithPreset(eng, DefaultTheme, PresetDashboard, nil)
	before := eng.Document().Revision
	_ = m.View()
	if _, ok := eng.NextEvent(); !ok {
		t.Fatal("expected initial committed event")
	}
	published := m.PublishCount()
	updated, _ := m.Update(AnimationTickMsg{Epoch: m.animationEpoch})
	m = updated.(Model)
	_ = m.View()
	if eng.Document().Revision != before {
		t.Fatal("animation changed document revision")
	}
	if m.PublishCount() != published {
		t.Fatalf("animation caused publish: %d -> %d", published, m.PublishCount())
	}
	if ev, ok := eng.NextEvent(); ok && ev.Ev == "committed" {
		t.Fatalf("animation emitted fake commit: %+v", ev)
	}
}

func TestStaticDocumentNeedsNoAnimation(t *testing.T) {
	eng := newTestEngine()
	m := NewModelWithPreset(eng, DefaultTheme, PresetMinimal, nil)
	if m.needsAnimation() {
		t.Fatal("empty static document should not animate")
	}
	if cmd := m.animationCommandIfNeeded(); cmd != nil {
		t.Fatal("static document scheduled animation")
	}
}

func lineContainsBoth(s, a, b string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, a) && strings.Contains(line, b) {
			return true
		}
	}
	return false
}

func TestExplicitBoxPresentationOverridesPresetDefaults(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "card", Type: protocol.NodeBox, Parent: "root",
		Props: json.RawMessage(`{"variant":"card"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "body", Type: protocol.NodeText, Parent: "card",
		Props: json.RawMessage(`{"text":"content"}`),
	}); err != nil {
		t.Fatal(err)
	}

	r := NewRendererWithPreset(DefaultTheme, PresetDashboard)
	doc := eng.Document()
	withPresetBorder := r.RenderTree(doc, "", nil, nil, 40, 10)
	if !strings.Contains(withPresetBorder, "╭") || !strings.Contains(withPresetBorder, "╯") {
		t.Fatalf("expected dashboard card rounded preset border, got:\n%s", withPresetBorder)
	}

	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 3, Op: protocol.OpProps, ID: "card",
		Props: json.RawMessage(`{"border":"none","padding":0,"gap":0}`),
	}); err != nil {
		t.Fatal(err)
	}
	doc = eng.Document()
	explicitNone := r.RenderTree(doc, "", nil, nil, 40, 10)
	if strings.ContainsAny(explicitNone, "╭╮╰╯") {
		t.Fatalf("explicit presentation props must override preset defaults, got:\n%s", explicitNone)
	}
}

func TestCompactTableNeverExceedsAvailableWidth(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "tbl", Type: protocol.NodeTable, Parent: "root",
		Props: json.RawMessage(`{
			"variant":"compact",
			"selectable":true,
			"columns":[
				{"title":"Node","width":14},
				{"title":"Role","width":16},
				{"title":"Status","width":12},
				{"title":"Load","width":9}
			],
			"rows":[["local-core","orchestrator","READY","18%"]]
		}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Focus("tbl"); err != nil {
		t.Fatal(err)
	}

	const maxW = 50
	view := NewRendererWithPreset(DefaultTheme, PresetMinimal).RenderTree(eng.Document(), "tbl", nil, map[string]int{"tbl": 0}, maxW, 20)
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > maxW {
			t.Fatalf("line %d width=%d exceeds %d: %q", i+1, got, maxW, line)
		}
	}
}

func TestColumnGapConsumesExactlyRequestedBlankLines(t *testing.T) {
	eng := newTestEngine()
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "col", Type: protocol.NodeBox, Parent: "root", Props: json.RawMessage(`{"dir":"col","gap":1}`)},
		{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "a", Type: protocol.NodeText, Parent: "col", Props: json.RawMessage(`{"text":"A"}`)},
		{V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "b", Type: protocol.NodeText, Parent: "col", Props: json.RawMessage(`{"text":"B"}`)},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	view := NewRendererWithPreset(DefaultTheme, PresetMinimal).RenderTree(eng.Document(), "", nil, nil, 20, 10)
	if view != "A\n\nB" {
		t.Fatalf("gap=1 must create exactly one blank line, got %q", view)
	}
}

func TestAnimationLifecycleStopsWhenSemanticNeedDisappears(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "spin-stop", Type: protocol.NodeProgress, Parent: "root",
		Props: json.RawMessage(`{"variant":"spinner","state":"loading","label":"Loading"}`),
	}); err != nil {
		t.Fatal(err)
	}
	m := NewModelWithPreset(eng, DefaultTheme, PresetDashboard, nil)
	if !m.animationActive || m.Init() == nil {
		t.Fatal("loading spinner must activate exactly one scheduler chain")
	}
	oldEpoch := m.animationEpoch

	if err := eng.Apply(protocol.Operation{
		V: 1, Seq: 2, Op: protocol.OpProps, ID: "spin-stop",
		Props: json.RawMessage(`{"state":"success"}`),
	}); err != nil {
		t.Fatal(err)
	}
	updated, cmd := m.Update(EngineDirtyMsg{})
	m = updated.(Model)
	if m.animationActive || cmd != nil {
		t.Fatalf("success state must stop animation: active=%v cmd=%v", m.animationActive, cmd != nil)
	}
	phase := m.animationPhase
	updated, cmd = m.Update(AnimationTickMsg{Epoch: oldEpoch})
	m = updated.(Model)
	if cmd != nil || m.animationPhase != phase {
		t.Fatal("stale animation tick resurrected a stopped animation chain")
	}
}

func TestFocusedInputControlsCursorAnimationNeed(t *testing.T) {
	eng := newTestEngine()
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "input-anim", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{}`)},
		{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "table-focus", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"columns":[{"title":"x","width":3}],"rows":[["1"]]}`)},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Focus("table-focus"); err != nil {
		t.Fatal(err)
	}
	m := NewModelWithPreset(eng, DefaultTheme, PresetMinimal, nil)
	if m.needsAnimation() {
		t.Fatal("unfocused input must not require cursor animation")
	}
	if err := eng.Focus("input-anim"); err != nil {
		t.Fatal(err)
	}
	if !m.needsAnimation() {
		t.Fatal("focused input must require renderer-local cursor animation")
	}
}

func TestParsePresetRejectsUnknownValue(t *testing.T) {
	if _, err := ParsePreset("neon-random"); err == nil {
		t.Fatal("unknown renderer preset must be rejected")
	}
}

func TestColumnAlignStretchPadsSiblingCanvasToCommonWidth(t *testing.T) {
	view := joinVertical("stretch", 0, []string{"A", "BBB"})
	lines := strings.Split(view, "\n")
	if len(lines) != 2 || lipgloss.Width(lines[0]) != 3 || lipgloss.Width(lines[1]) != 3 {
		t.Fatalf("stretch must equalize cross-axis width, got %q", view)
	}
}

func TestBubbleTeaTableEnterEmitsSelectInsteadOfSubmit(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 200, Op: protocol.OpUpsert, ID: "jobs", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"action":"open_job","columns":[{"title":"Job","width":10}],"rows":[["A"],["B"]],"row_ids":["job-a","job-b"]}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Focus("jobs"); err != nil {
		t.Fatal(err)
	}
	m := NewModel(eng, DefaultTheme, nil)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	ev, ok := eng.NextEvent()
	if !ok || ev.Ev != "select" || ev.RowID != "job-b" || ev.Action != "open_job" {
		t.Fatalf("select event=%+v ok=%v", ev, ok)
	}
}

func TestBubbleTeaScrollableViewportParticipatesInTabOrder(t *testing.T) {
	eng := newTestEngine()
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 210, Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{}`)},
		{V: 1, Seq: 211, Op: protocol.OpUpsert, ID: "passive", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"scrollable":false}`)},
		{V: 1, Seq: 212, Op: protocol.OpUpsert, ID: "logs", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"scrollable":true}`)},
		{V: 1, Seq: 213, Op: protocol.OpUpsert, ID: "tbl", Type: protocol.NodeTable, Parent: "root", Props: json.RawMessage(`{"selectable":true,"rows":[["x"]]}`)},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Focus("in"); err != nil {
		t.Fatal(err)
	}
	m := NewModel(eng, DefaultTheme, nil)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if got := eng.FocusedID(); got != "logs" {
		t.Fatalf("focus=%q want logs", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	if got := eng.FocusedID(); got != "tbl" {
		t.Fatalf("focus=%q want tbl", got)
	}
}

func TestFocusedInputConsumesActionHotkeyAndSupportsRuneSafeCaretEditing(t *testing.T) {
	triggered := false
	reg := a2runtime.NewActionRegistry(time.Second, 2)
	if err := reg.Register("restart", func(context.Context, json.RawMessage) error { triggered = true; return nil }); err != nil {
		t.Fatal(err)
	}
	eng := engine.New(protocol.DefaultLimits(), 16, reg)
	for _, op := range []protocol.Operation{
		{V: 1, Seq: 220, Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"Ж🙂","action":"submit"}`)},
		{V: 1, Seq: 221, Op: protocol.OpUpsert, ID: "actions", Type: protocol.NodeActions, Parent: "root", Props: json.RawMessage(`{"items":[{"key":"r","label":"Restart","action":"restart"}]}`)},
	} {
		if err := eng.Apply(op); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.Focus("in"); err != nil {
		t.Fatal(err)
	}
	m := NewModel(eng, DefaultTheme, nil)

	// Caret begins at end. Move before emoji, insert r (which must NOT trigger action),
	// delete emoji, then move home and insert a Cyrillic rune.
	for _, msg := range []tea.KeyMsg{
		{Type: tea.KeyLeft},
		{Type: tea.KeyRunes, Runes: []rune{'r'}},
		{Type: tea.KeyDelete},
		{Type: tea.KeyHome},
		{Type: tea.KeyRunes, Runes: []rune{'Я'}},
		{Type: tea.KeyEnd},
	} {
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	if triggered {
		t.Fatal("focused input leaked rune to global action binding")
	}
	if got := eng.InputValue("in"); got != "ЯЖr" {
		t.Fatalf("input=%q want %q", got, "ЯЖr")
	}
	if caret := m.interaction.InputCarets["in"]; caret != len([]rune("ЯЖr")) {
		t.Fatalf("caret=%d", caret)
	}
}

func TestViewportManualScrollUnpinsAndEndRepinsFollowTail(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 230, Op: protocol.OpUpsert, ID: "logs", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"height":3,"wrap":true,"follow_tail":true,"scrollable":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 231, Op: protocol.OpUpsert, ID: "logtext", Type: protocol.NodeText, Parent: "logs", Props: json.RawMessage(`{"text":"1\n2\n3\n4\n5\n6"}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Focus("logs"); err != nil {
		t.Fatal(err)
	}
	m := NewModel(eng, DefaultTheme, nil)
	result := m.renderResult()
	if got := result.Viewports["logs"].Offset; got != 3 {
		t.Fatalf("initial tail offset=%d want 3", got)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(Model)
	state := m.interaction.Viewports["logs"]
	if state.PinnedToTail || state.Offset >= 3 {
		t.Fatalf("page up did not unpin: %+v", state)
	}
	readingOffset := state.Offset

	if err := eng.Apply(protocol.Operation{V: 1, Seq: 232, Op: protocol.OpText, ID: "logtext", Text: "\n7\n8"}); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(EngineDirtyMsg{})
	m = updated.(Model)
	if got := m.renderResult().Viewports["logs"].Offset; got != readingOffset {
		t.Fatalf("agent append yanked unpinned viewport: %d -> %d", readingOffset, got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(Model)
	state = m.interaction.Viewports["logs"]
	if !state.PinnedToTail {
		t.Fatalf("End did not repin: %+v", state)
	}
	beforeTail := m.renderResult().Viewports["logs"].Offset
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 233, Op: protocol.OpText, ID: "logtext", Text: "\n9"}); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(EngineDirtyMsg{})
	m = updated.(Model)
	afterTail := m.renderResult().Viewports["logs"].Offset
	if afterTail <= beforeTail {
		t.Fatalf("pinned viewport did not follow tail: %d -> %d", beforeTail, afterTail)
	}
}

func TestViewportMetricsRecomputeAfterWidthChange(t *testing.T) {
	eng := newTestEngine()
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 240, Op: protocol.OpUpsert, ID: "logs", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"height":2,"wrap":true,"scrollable":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Apply(protocol.Operation{V: 1, Seq: 241, Op: protocol.OpUpsert, ID: "logtext", Type: protocol.NodeText, Parent: "logs", Props: json.RawMessage(`{"text":"abcdefghijklmnopqrst"}`)}); err != nil {
		t.Fatal(err)
	}
	m := NewModel(eng, DefaultTheme, nil)
	m.Width = 20
	wide := m.renderResult().Viewports["logs"]
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 5, Height: 20})
	m = updated.(Model)
	narrow := m.renderResult().Viewports["logs"]
	if narrow.TotalLines <= wide.TotalLines || narrow.MaxOffset <= wide.MaxOffset {
		t.Fatalf("metrics did not respond to wrapping: wide=%+v narrow=%+v", wide, narrow)
	}
}

func TestInputCaretClampsAfterForcedAgentValueAndLocalStatePrunes(t *testing.T) {
	eng := newTestEngine()
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 1, Op: protocol.OpUpsert, ID: "in", Type: protocol.NodeInput, Parent: "root", Props: json.RawMessage(`{"value":"abcdef"}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 2, Op: protocol.OpUpsert, ID: "vp", Type: protocol.NodeViewport, Parent: "root", Props: json.RawMessage(`{"height":2,"scrollable":true}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 3, Op: protocol.OpUpsert, ID: "vpt", Type: protocol.NodeText, Parent: "vp", Props: json.RawMessage(`{"text":"one\ntwo\nthree"}`)}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.Focus("in"); perr != nil {
		t.Fatal(perr)
	}
	model := NewModel(eng, DefaultTheme, nil)
	model.interaction.InputCarets["in"] = 5
	model.interaction.Viewports["vp"] = ViewportState{Offset: 1}

	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 4, Op: protocol.OpProps, ID: "in", Props: json.RawMessage(`{"value":"Ж","force":true}`)}); perr != nil {
		t.Fatal(perr)
	}
	model.reconcileLocalState()
	if got := model.interaction.InputCarets["in"]; got != 1 {
		t.Fatalf("forced shorter value must clamp caret to rune length 1, got %d", got)
	}

	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 5, Op: protocol.OpRemove, ID: "in"}); perr != nil {
		t.Fatal(perr)
	}
	if perr := eng.Apply(protocol.Operation{V: 1, Seq: 6, Op: protocol.OpProps, ID: "vp", Props: json.RawMessage(`{"scrollable":false}`)}); perr != nil {
		t.Fatal(perr)
	}
	model.reconcileLocalState()
	if _, ok := model.interaction.InputCarets["in"]; ok {
		t.Fatal("deleted input caret state was not pruned")
	}
	if _, ok := model.interaction.Viewports["vp"]; ok {
		t.Fatal("non-scrollable viewport local state was not pruned")
	}
}
