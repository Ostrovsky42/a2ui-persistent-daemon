package e2e

import (
	"strings"
	"testing"

	a2tea "a2ui/adapter/bubbletea"
	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInteractiveRuntimeAgentUserRoundTripAcrossPresets(t *testing.T) {
	for _, preset := range []a2tea.Preset{a2tea.PresetMinimal, a2tea.PresetDashboard, a2tea.PresetDense} {
		t.Run(string(preset), func(t *testing.T) {
			runner := NewRunner("interactive-"+string(preset), protocol.DefaultLimits(), 120, 40, nil)
			agent := NewAgentSimulator("interactive-"+string(preset), runner)
			if err := agent.Connect(); err != nil {
				t.Fatal(err)
			}
			setupInteractiveRuntimeDocument(t, agent)
			_, _ = agent.FetchEvents() // discard initial commit acknowledgement

			model := a2tea.NewModelWithPreset(runner.Engine, a2tea.DefaultTheme, preset, nil)
			model.Width, model.Height = 120, 40
			if perr := runner.Engine.Focus("jobs"); perr != nil {
				t.Fatal(perr)
			}
			model = updateModel(t, model, a2tea.EngineDirtyMsg{})

			model = updateModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
			model = updateModel(t, model, tea.KeyMsg{Type: tea.KeyDown})
			model = updateModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
			events, err := agent.FetchEvents()
			if err != nil {
				t.Fatal(err)
			}
			selectEvent := findEvent(events, "select")
			if selectEvent == nil {
				t.Fatalf("expected select event, got %+v", events)
			}
			if selectEvent.ID != "jobs" || selectEvent.Row != 2 || selectEvent.RowID != "job-c" || selectEvent.Action != "open_job" {
				t.Fatalf("unexpected select event: %+v", *selectEvent)
			}

			if err := agent.UpdateProps("jobs", map[string]any{
				"rows": [][]string{
					{"job-c", "running"},
					{"job-a", "queued"},
					{"job-b", "done"},
				},
				"row_ids": []string{"job-c", "job-a", "job-b"},
			}); err != nil {
				t.Fatal(err)
			}
			if err := agent.UpdateProps("details", map[string]any{"text": "Selected: job-c"}); err != nil {
				t.Fatal(err)
			}
			if err := agent.AppendText("log-text", "\nL9 job-c selected\nL10 details refreshed"); err != nil {
				t.Fatal(err)
			}
			if err := agent.Commit("job-c-selected"); err != nil {
				t.Fatal(err)
			}
			model = updateModel(t, model, a2tea.EngineDirtyMsg{})
			if sel, ok := runner.Engine.SelectedTableRow("jobs"); !ok || sel.RowID != "job-c" || sel.Index != 0 {
				t.Fatalf("selection did not follow stable row ID after reorder: %+v ok=%v", sel, ok)
			}
			if frame := model.View(); !strings.Contains(frame, "Selected: job-c") {
				t.Fatalf("agent update missing from rendered frame:\n%s", frame)
			}
			_, _ = agent.FetchEvents() // discard commit acknowledgement

			if perr := runner.Engine.Focus("logs"); perr != nil {
				t.Fatal(perr)
			}
			model = updateModel(t, model, a2tea.EngineDirtyMsg{})
			model = updateModel(t, model, tea.KeyMsg{Type: tea.KeyPgUp})
			beforeAppend := model.View()
			if !strings.Contains(beforeAppend, "L6") {
				t.Fatalf("expected manually scrolled viewport to expose earlier logs:\n%s", beforeAppend)
			}

			if err := agent.AppendText("log-text", "\nL11 background update\nL12 still running"); err != nil {
				t.Fatal(err)
			}
			if err := agent.Commit("logs-appended-unpinned"); err != nil {
				t.Fatal(err)
			}
			model = updateModel(t, model, a2tea.EngineDirtyMsg{})
			unpinned := model.View()
			if !strings.Contains(unpinned, "L6") || strings.Contains(unpinned, "L12 still running") {
				t.Fatalf("agent append yanked manually scrolled viewport to tail:\n%s", unpinned)
			}
			_, _ = agent.FetchEvents()

			model = updateModel(t, model, tea.KeyMsg{Type: tea.KeyEnd})
			atTail := model.View()
			if !strings.Contains(atTail, "L12 still running") {
				t.Fatalf("End did not repin viewport to tail:\n%s", atTail)
			}
			if err := agent.AppendText("log-text", "\nL13 tail-follow proof"); err != nil {
				t.Fatal(err)
			}
			if err := agent.Commit("logs-appended-pinned"); err != nil {
				t.Fatal(err)
			}
			model = updateModel(t, model, a2tea.EngineDirtyMsg{})
			if frame := model.View(); !strings.Contains(frame, "L13 tail-follow proof") {
				t.Fatalf("pinned viewport did not follow appended tail:\n%s", frame)
			}
			_, _ = agent.FetchEvents()

			if perr := runner.Engine.Focus("command"); perr != nil {
				t.Fatal(perr)
			}
			model = updateModel(t, model, a2tea.EngineDirtyMsg{})
			for _, msg := range []tea.KeyMsg{
				{Type: tea.KeyRunes, Runes: []rune("Ж🙂")},
				{Type: tea.KeyLeft},
				{Type: tea.KeyRunes, Runes: []rune{'r'}},
				{Type: tea.KeyDelete},
				{Type: tea.KeyHome},
				{Type: tea.KeyRunes, Runes: []rune{'Я'}},
				{Type: tea.KeyEnd},
			} {
				model = updateModel(t, model, msg)
			}
			if got := runner.Engine.InputValue("command"); got != "ЯЖr" {
				t.Fatalf("unexpected rune-safe edited value %q", got)
			}
			model = updateModel(t, model, tea.KeyMsg{Type: tea.KeyEnter})
			events, err = agent.FetchEvents()
			if err != nil {
				t.Fatal(err)
			}
			submit := findEvent(events, "submit")
			if submit == nil || submit.ID != "command" || submit.Value != "ЯЖr" {
				t.Fatalf("unexpected submit after caret editing: %+v events=%+v", submit, events)
			}

			model = updateModel(t, model, tea.WindowSizeMsg{Width: 50, Height: 30})
			narrow := model.View()
			if !strings.Contains(narrow, "Selected: job-c") {
				t.Fatalf("narrow frame lost semantic content:\n%s", narrow)
			}
			model = updateModel(t, model, tea.WindowSizeMsg{Width: 120, Height: 40})
			_ = model.View()
			if sel, ok := runner.Engine.SelectedTableRow("jobs"); !ok || sel.RowID != "job-c" {
				t.Fatalf("resize lost stable table selection: %+v ok=%v", sel, ok)
			}
			if got := runner.Engine.InputValue("command"); got != "ЯЖr" {
				t.Fatalf("resize lost input value: %q", got)
			}
		})
	}
}

func setupInteractiveRuntimeDocument(t *testing.T, agent *AgentSimulator) {
	t.Helper()
	upserts := []struct {
		id, parent string
		typ        protocol.NodeType
		props      map[string]any
	}{
		{"shell", "root", protocol.NodeBox, map[string]any{"dir": "col", "gap": 1}},
		{"title", "shell", protocol.NodeText, map[string]any{"text": "Interactive Ops", "variant": "title"}},
		{"main", "shell", protocol.NodeBox, map[string]any{"dir": "row", "responsive": "stack", "gap": 1}},
		{"jobs", "main", protocol.NodeTable, map[string]any{
			"selectable": true,
			"action":     "open_job",
			"row_ids":    []string{"job-a", "job-b", "job-c"},
			"columns": []map[string]any{
				{"title": "Job", "width": 16},
				{"title": "State", "width": 10},
			},
			"rows": [][]string{{"job-a", "queued"}, {"job-b", "done"}, {"job-c", "running"}},
			"flex": map[string]any{"grow": 1, "basis": 30, "min_width": 24},
		}},
		{"details-card", "main", protocol.NodeBox, map[string]any{"variant": "card", "dir": "col", "flex": map[string]any{"grow": 1, "basis": 30, "min_width": 24}}},
		{"details", "details-card", protocol.NodeText, map[string]any{"text": "Selected: none", "variant": "body"}},
		{"progress", "details-card", protocol.NodeProgress, map[string]any{"variant": "compact", "state": "loading", "label": "Agent working"}},
		{"logs", "shell", protocol.NodeViewport, map[string]any{"height": 3, "wrap": true, "follow_tail": true, "scrollable": true}},
		{"log-text", "logs", protocol.NodeText, map[string]any{"variant": "code", "text": "L1\nL2\nL3\nL4\nL5\nL6\nL7\nL8"}},
		{"command", "shell", protocol.NodeInput, map[string]any{"value": "", "placeholder": "Command", "action": "run_command"}},
		{"actions", "shell", protocol.NodeActions, map[string]any{"variant": "toolbar", "items": []map[string]any{{"key": "r", "label": "Restart", "action": "restart"}}}},
	}
	for _, u := range upserts {
		if err := agent.Upsert(u.id, u.parent, u.typ, u.props, nil); err != nil {
			t.Fatalf("upsert %s: %v", u.id, err)
		}
	}
	if err := agent.Commit("interactive-initial"); err != nil {
		t.Fatal(err)
	}
}

func updateModel(t *testing.T, model a2tea.Model, msg tea.Msg) a2tea.Model {
	t.Helper()
	updated, _ := model.Update(msg)
	next, ok := updated.(a2tea.Model)
	if !ok {
		t.Fatalf("unexpected Bubble Tea model type %T", updated)
	}
	return next
}

func findEvent(events []protocol.Event, kind string) *protocol.Event {
	for i := range events {
		if events[i].Ev == kind {
			return &events[i]
		}
	}
	return nil
}
