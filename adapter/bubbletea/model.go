package bubbletea

import (
	"sync"
	"sync/atomic"
	"time"

	"a2ui/engine"
	"a2ui/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

// EngineDirtyMsg signals that the engine document or projection has been updated.
type EngineDirtyMsg struct{}

// AnimationTickMsg advances renderer-local animation only. Epoch prevents stale
// timer chains from resurrecting animation after the semantic state becomes static.
type AnimationTickMsg struct{ Epoch uint64 }

const animationInterval = 120 * time.Millisecond

// Model is the Bubble Tea adapter for an A2UI Engine. Semantic interaction
// state lives in Engine/runtime; interaction contains only terminal-local
// mechanics such as caret and viewport offset.
type Model struct {
	Engine       *engine.Engine
	controller   SemanticController
	Renderer     *Renderer
	Width        int
	Height       int
	interaction  InteractionState
	updateChan   <-chan struct{}
	Quitting     bool
	publishCount *atomic.Uint64
	ackMu        *sync.Mutex
	lastAck      *atomic.Uint64

	animationPhase  uint64
	animationEpoch  uint64
	animationActive bool
	cursorVisible   bool
}

// NewModel preserves the original API and uses the minimal preset.
func NewModel(eng *engine.Engine, theme Theme, updateChan <-chan struct{}) Model {
	return NewModelWithPreset(eng, theme, PresetMinimal, updateChan)
}

// NewModelWithPreset creates an initialized Bubble Tea adapter with an explicit
// renderer-side presentation preset.
func NewModelWithPreset(eng *engine.Engine, theme Theme, preset Preset, updateChan <-chan struct{}) Model {
	m := NewModelWithController(localSemanticController{eng: eng}, theme, preset, updateChan)
	m.Engine = eng // Backward-compatible local runner/test access only.
	return m
}

// NewModelWithController creates a Bubble Tea model backed by a semantic
// controller. Remote controllers must not install a local authoritative Engine.
func NewModelWithController(controller SemanticController, theme Theme, preset Preset, updateChan <-chan struct{}) Model {
	m := Model{
		controller:    controller,
		Renderer:      NewRendererWithPreset(theme, preset),
		Width:         80,
		Height:        24,
		interaction:   newInteractionState(),
		updateChan:    updateChan,
		publishCount:  new(atomic.Uint64),
		ackMu:         new(sync.Mutex),
		lastAck:       new(atomic.Uint64),
		cursorVisible: true,
	}
	m.reconcileLocalState()
	m.animationActive = m.needsAnimation()
	return m
}

// PublishCount returns the number of publication barriers executed by View().
func (m Model) PublishCount() uint64 {
	if m.publishCount != nil {
		return m.publishCount.Load()
	}
	return 0
}

func waitForUpdate(ch <-chan struct{}) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		_, ok := <-ch
		if !ok {
			return nil
		}
		return EngineDirtyMsg{}
	}
}

func animationTickCmd(epoch uint64) tea.Cmd {
	return tea.Tick(animationInterval, func(time.Time) tea.Msg { return AnimationTickMsg{Epoch: epoch} })
}

// Init starts external update listening and, only when required by the current
// presentation state, one animation timer chain.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if cmd := waitForUpdate(m.updateChan); cmd != nil {
		cmds = append(cmds, cmd)
	}
	if m.animationActive {
		cmds = append(cmds, animationTickCmd(m.animationEpoch))
	}
	return batchCommands(cmds...)
}

func batchCommands(cmds ...tea.Cmd) tea.Cmd {
	filtered := cmds[:0]
	for _, cmd := range cmds {
		if cmd != nil {
			filtered = append(filtered, cmd)
		}
	}
	switch len(filtered) {
	case 0:
		return nil
	case 1:
		return filtered[0]
	default:
		return tea.Batch(filtered...)
	}
}

// Update processes Bubble Tea messages, user input, layout changes and the
// single renderer-local animation chain.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.reconcileLocalState()
		return m, nil

	case EngineDirtyMsg:
		m.reconcileLocalState()
		anim := m.animationCommandIfNeeded()
		return m, batchCommands(waitForUpdate(m.updateChan), anim)

	case AnimationTickMsg:
		if !m.animationActive || msg.Epoch != m.animationEpoch {
			return m, nil
		}
		if !m.needsAnimation() {
			m.stopAnimation()
			return m, nil
		}
		m.animationPhase++
		// Cursor blinks more slowly than the spinner while sharing the same
		// scheduler. No document/runtime mutation occurs.
		m.cursorVisible = (m.animationPhase/3)%2 == 0
		return m, animationTickCmd(m.animationEpoch)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) semanticSnapshot() engine.PresentationSnapshot {
	if m.controller == nil {
		return engine.PresentationSnapshot{}
	}
	return m.controller.Snapshot()
}

func (m Model) renderResultForSnapshot(snapshot engine.PresentationSnapshot) RenderResult {
	if m.controller == nil || m.Renderer == nil {
		return RenderResult{Viewports: map[string]ViewportMetrics{}}
	}
	return m.Renderer.RenderFrame(
		snapshot.Document,
		snapshot.FocusedID,
		snapshot.InputValues,
		snapshot.TableSelections,
		m.interaction,
		m.Width,
		m.Height,
		RenderState{AnimationPhase: m.animationPhase, CursorVisible: m.cursorVisible},
	)
}

func (m Model) renderResult() RenderResult {
	return m.renderResultForSnapshot(m.semanticSnapshot())
}

func (m Model) View() string {
	if m.Quitting {
		return "Session terminated.\n"
	}
	snapshot := m.semanticSnapshot()
	out := m.renderResultForSnapshot(snapshot).Frame
	connectionErr := semanticControllerError(m.controller)
	if connectionErr != nil {
		if out != "" {
			out += "\n"
		}
		out += "Daemon disconnected: " + connectionErr.Error() + "\n"
	}

	// Acknowledge exactly the publication generation that produced this visible
	// frame. A remote controller forwards this ACK to the daemon; rendering a
	// newer snapshot can therefore never accidentally publish an older frame.
	if connectionErr == nil && snapshot.PublicationPending && snapshot.PublicationGeneration != 0 && m.controller != nil && m.ackMu != nil && m.lastAck != nil {
		m.ackMu.Lock()
		if m.lastAck.Load() != snapshot.PublicationGeneration {
			if err := m.controller.AcknowledgePublication(snapshot.PublicationGeneration); err == nil {
				m.lastAck.Store(snapshot.PublicationGeneration)
				if m.publishCount != nil {
					m.publishCount.Add(1)
				}
			}
		}
		m.ackMu.Unlock()
	}
	return out
}

func (m Model) needsAnimation() bool {
	if m.controller == nil || m.Renderer == nil {
		return false
	}
	snapshot := m.semanticSnapshot()
	return m.Renderer.HasActiveAnimation(snapshot.Document, snapshot.FocusedID)
}

// animationCommandIfNeeded transitions from static to animated presentation.
// It returns nil while an animation chain is already active, preventing timer
// fan-out when several engine updates arrive close together.
func (m *Model) animationCommandIfNeeded() tea.Cmd {
	need := m.needsAnimation()
	if !need {
		if m.animationActive {
			m.stopAnimation()
		}
		return nil
	}
	if m.animationActive {
		return nil
	}
	m.animationActive = true
	m.cursorVisible = true
	m.animationEpoch++
	return animationTickCmd(m.animationEpoch)
}

func (m *Model) stopAnimation() {
	if m.animationActive {
		m.animationActive = false
		m.animationEpoch++
	}
	m.cursorVisible = true
}

func (m *Model) reconcileLocalState() {
	if m.controller == nil || m.Renderer == nil {
		return
	}
	m.interaction = m.interaction.clone()
	snapshot := m.semanticSnapshot()

	for id := range m.interaction.InputCarets {
		n, ok := snapshot.Document.Nodes[id]
		if !ok || n.Type != protocol.NodeInput {
			delete(m.interaction.InputCarets, id)
		}
	}
	for id, n := range snapshot.Document.Nodes {
		if n.Type != protocol.NodeInput {
			continue
		}
		maxCaret := len([]rune(snapshot.InputValues[id]))
		caret, exists := m.interaction.InputCarets[id]
		if !exists {
			caret = maxCaret
		}
		if caret < 0 {
			caret = 0
		}
		if caret > maxCaret {
			caret = maxCaret
		}
		m.interaction.InputCarets[id] = caret
	}

	for id := range m.interaction.Viewports {
		n, ok := snapshot.Document.Nodes[id]
		if !ok || n.Type != protocol.NodeViewport || !propBool(n, "scrollable", false) {
			delete(m.interaction.Viewports, id)
		}
	}
	for id, n := range snapshot.Document.Nodes {
		if n.Type != protocol.NodeViewport || !propBool(n, "scrollable", false) {
			continue
		}
		if _, exists := m.interaction.Viewports[id]; !exists {
			m.interaction.Viewports[id] = ViewportState{PinnedToTail: propBool(n, "follow_tail", false)}
		}
	}

	// A table window is not a second user-controlled scroll state. Its top row is
	// adapter-local presentation continuity state: the previous offset participates
	// in reconciliation together with runtime-owned selection and current geometry.
	// The renderer constrains/clamps that state to a valid fixed point; it does not
	// derive it history-free. Removal/non-selectability prunes the local state.
	for id := range m.interaction.TableViewports {
		n, ok := snapshot.Document.Nodes[id]
		if !ok || n.Type != protocol.NodeTable || !propBool(n, "selectable", false) {
			delete(m.interaction.TableViewports, id)
		}
	}

	// Let the renderer resolve wrapped visual-line and table-row geometry, then
	// clamp local offsets without duplicating layout calculations here.
	result := m.Renderer.RenderFrame(
		snapshot.Document,
		snapshot.FocusedID,
		snapshot.InputValues,
		snapshot.TableSelections,
		m.interaction,
		m.Width,
		m.Height,
		RenderState{AnimationPhase: m.animationPhase, CursorVisible: m.cursorVisible},
	)
	for id, local := range m.interaction.Viewports {
		metrics, ok := result.Viewports[id]
		if !ok {
			continue
		}
		local.Offset = metrics.Offset
		m.interaction.Viewports[id] = local
	}
	for id, metrics := range result.Tables {
		n, ok := snapshot.Document.Nodes[id]
		if !ok || n.Type != protocol.NodeTable || !propBool(n, "selectable", false) {
			continue
		}
		m.interaction.TableViewports[id] = TableViewportState{Offset: metrics.Offset}
	}
}
