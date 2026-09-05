# A2UI V3 Interactive Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add renderer-independent table selection, manual viewport navigation, rune-safe input caret editing and coherent interactive rendering without weakening A2UI publication or authority laws.

**Architecture:** Document remains agent-owned semantic state. Runtime projection owns focus, input values and semantic table selection. Bubble Tea owns only caret/viewport/animation/terminal mechanics. Engine separates local redraw generation from agent publication generation and exposes one atomic presentation snapshot.

**Tech Stack:** Go 1.24.0, Bubble Tea 1.3.10, Lip Gloss 1.1.0, existing A2UI v1 core and MCP simulator.

**Spec:** `docs/superpowers/specs/2026-09-05-a2ui-v3-interactive-runtime-design.md`

## Global Constraints

- Preserve the V1 transactional Document and V2 presentation contract.
- Local focus/input/table interaction MUST NOT create protocol publication acknowledgements.
- Viewport offset, input caret, animation phase and terminal dimensions MUST remain adapter-local.
- Stable table identity uses optional unique non-empty `row_ids` parallel to `rows`.
- `select` is a critical event and explicit backpressure is mandatory.
- Key routing is process control -> focus traversal -> focused component -> global action.
- No mouse, Web renderer, remote per-keystroke events or transport redesign in V3.
- Do not downgrade the repository `go 1.24.0` requirement for a constrained sandbox.

---

### Task 1: Separate local redraw from protocol publication

**Files:** `runtime/state.go`, `runtime/publication.go`, `engine/engine.go`, `engine/engine_test.go`

**Produces:** independent render/publication generations and local-focus semantics.

- [x] Characterize local input/focus currently making publication dirty.
- [x] Add publication generation separate from render generation.
- [x] Keep agent `OpFocus` publication-aware while `Engine.Focus` is local-only.
- [x] Prove local input/focus do not make `Engine.NeedsPublish()` true.

### Task 2: Add strict V3 protocol props

**Files:** `protocol/props.go`, `protocol/types.go`, `protocol/protocol_test.go`, `assets/schema.json`, `protocol/schema_contract_test.go`

**Produces:** `viewport.scrollable`, table `row_ids`/`action`, event `row_id`.

- [x] Add strict validation for booleans, string action and row ID arrays.
- [x] Reject empty/duplicate IDs and row-count mismatch transactionally.
- [x] Add `row_id` to protocol events.
- [x] Synchronize Draft 2020-12 schema and contract tests.

### Task 3: Move table selection into Runtime projection

**Files:** `runtime/table_selection.go`, `runtime/state.go`, `runtime/runtime_test.go`, `engine/engine.go`, `engine/engine_test.go`

**Produces:** `TableSelection`, movement/reconciliation/activation APIs.

- [x] Initialize selection only for non-empty selectable tables.
- [x] Preserve stable RowID through row reorder.
- [x] Clamp/remove selection on data/capability/type/removal changes.
- [x] Ensure local selection is redraw-only, not publication dirty.
- [x] Emit critical `select` with row, row_id and action.
- [x] Prove critical broker backpressure is explicit.

### Task 4: Add atomic presentation snapshots

**Files:** `engine/engine.go`, `engine/engine_test.go`

**Produces:** `PresentationSnapshot` with cloned Document, focus, input values and table selections.

- [x] Read snapshot under one Engine lock.
- [x] Return copies that cannot mutate Engine internals.
- [x] Migrate Bubble Tea rendering to one snapshot per frame.

### Task 5: Add adapter-local interaction state and renderer metrics

**Files:** `adapter/bubbletea/interaction_state.go`, `adapter/bubbletea/render_result.go`, `adapter/bubbletea/renderer.go`, `adapter/bubbletea/render_components.go`

**Produces:** bounded caret/viewport local state plus `RenderResult.Viewports` metrics.

- [x] Preserve V2 public renderer APIs as wrappers.
- [x] Render table selection only from Runtime snapshot.
- [x] Render caret from rune-indexed adapter state.
- [x] Compute viewport metrics after wrapping and clipping.
- [x] Keep renderer stateless.

### Task 6: Implement deterministic key routing and controllers

**Files:** `adapter/bubbletea/model.go`, `adapter/bubbletea/key_router.go`, `adapter/bubbletea/adapter_test.go`

**Produces:** focus traversal, table navigation, manual viewport scroll and caret editing.

- [x] Route Ctrl+C, then Tab traversal, then focused component, then actions.
- [x] Implement table Up/Down/Home/End/Enter.
- [x] Implement viewport Up/Down/PageUp/PageDown/Home/End with tail pin law.
- [x] Implement input Left/Right/Home/End/Backspace/Delete/rune insertion/Enter.
- [x] Prove focused input consumes action hotkeys.
- [x] Prune/clamp adapter-local state after agent changes.

### Task 7: Preserve local redraw in the integration runner

**Files:** `e2e/runner.go`, `e2e/e2e_test.go`

**Produces:** local input frames without calling `Publish()`.

- [x] Characterize stale frame after publication/local-redraw split.
- [x] Add local render path separate from publication hook/revision tracking.
- [x] Keep existing form E2E behavior green.

### Task 8: Add dynamic agent/user E2E and CLI scenario

**Files:** `e2e/interactive_runtime_test.go`, `cmd/a2ui-runner/interactive.go`, `cmd/a2ui-runner/interactive_test.go`, `cmd/a2ui-runner/main.go`

**Produces:** one serious V3 interaction story under all three V2 presets.

- [x] Drive initial Document through MCP AgentSimulator.
- [x] Select a stable row and verify `select` event.
- [x] Reorder rows from agent and preserve selection by RowID.
- [x] Unpin viewport, append logs, preserve reading position, repin and follow tail.
- [x] Perform Unicode caret editing and submit.
- [x] Resize narrow/wide without losing semantic interaction state.
- [x] Expose `-scenario interactive` for manual TUI use.

### Task 9: Synchronize normative documentation

**Files:** `README.md`, `SKILL.md`, `references/PROTOCOL.md`, `references/IMPLEMENTATION.md`, `references/MIGRATION.md`

**Produces:** one documented ownership and interaction law.

- [x] Document Document/Runtime/adapter-local ownership split.
- [x] Document stable row IDs/select events/manual viewport behavior.
- [x] Document publication versus local redraw separation.
- [x] Document atomic PresentationSnapshot and key-routing priority.

### Task 10: Release verification and artifact

**Files:** repository-wide verification only; no production behavior should be added here.

- [x] Run formatting and `git diff --check`.
- [x] Validate shipped examples against JSON Schema.
- [x] Run pure core tests/vet on the available toolchain mirror.
- [x] Run full API-compatible dependency harness tests/vet/race.
- [x] Run existing wire/document fuzz smoke.
- [x] Record literal Go 1.24 gate outcome without downgrading repository metadata.
- [ ] Commit exact V3 tree, create archive from commit, unpack and independently reverify artifact.
