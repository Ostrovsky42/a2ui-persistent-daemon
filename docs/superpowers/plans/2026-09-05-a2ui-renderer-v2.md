# A2UI Renderer V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add deterministic semantic presentation, responsive layout, three renderer-side presets and event-driven Bubble Tea motion without changing A2UI authority/publication semantics.

**Architecture:** The Document remains authoritative, runtime remains the interactive projection, and Bubble Tea owns only ephemeral presentation state. Semantic variants and bounded flex hints are protocol data; preset/theme and animation phase are renderer-local. Responsive stacking transforms presentation only.

**Tech Stack:** Go 1.24.0, Bubble Tea 1.3.10, Lip Gloss 1.1.0, existing stdlib-only A2UI semantic core.

**Spec:** `docs/superpowers/specs/2026-09-05-a2ui-renderer-v2-design.md`

## Global Constraints

- Preserve `document.Document` as sole authoritative semantic state.
- Preserve existing `Engine.Publish()` commit barrier and exactly-once acknowledgement semantics.
- Preserve legacy documents and existing public constructors.
- Presets are renderer-side only: `minimal`, `dashboard`, `dense`.
- No perpetual animation loop; one bounded event-driven scheduler per Bubble Tea Model.
- No CSS engine, Web renderer, manual viewport scrolling, percentage/absolute sizing, raw ANSI or arbitrary RGB.
- Failed protocol/document mutations must make zero authoritative change.

---

### Task 1: Correct viewport container ownership

**Files:**
- Modify: `protocol/types.go`
- Modify/Test: `document/reducer_test.go`

**Interfaces:**
- Consumes: `protocol.NodeType.Container()` and existing reducer structural validation.
- Produces: `NodeViewport.Container() == true` while all existing leaf node types remain false.

- [x] Add a reducer test creating `box -> viewport -> text` and assert success.
- [x] Add leaf-parent rejection assertions for text/table/input/actions/progress.
- [x] Run the focused test and confirm RED at `document.parent_not_container` for viewport.
- [x] Change `NodeType.Container()` to accept only box and viewport.
- [x] Run focused protocol/document tests and confirm GREEN.

### Task 2: Add strict presentation props and schema

**Files:**
- Modify: `protocol/props.go`
- Modify/Test: `protocol/protocol_test.go`
- Modify: `assets/schema.json`

**Interfaces:**
- Consumes: existing strict prop normalization and schema contract.
- Produces: semantic variants, progress state, box alignment/responsiveness, viewport follow-tail and common strict flex object.

- [x] Add RED tests for all valid new enums and invalid enum/flex forms.
- [x] Add backward-compatibility tests proving deterministic defaults for legacy props.
- [x] Implement strict defaults and enum validation in `NormalizeProps`.
- [x] Implement strict `flex` validation with grow/basis/min/max constraints.
- [x] Mirror the same contract in Draft 2020-12 `assets/schema.json`.
- [x] Validate shipped NDJSON examples against the updated schema.

### Task 3: Build deterministic flex allocator

**Files:**
- Create: `layout/flex.go`
- Modify/Test: `layout/layout_test.go`

**Interfaces:**
- Produces: `FlexConstraint` and `AllocateRow(available, gap int, children []FlexConstraint) ([]int, bool)`.

- [x] Write RED tests for equal allocation, grow 1/1, grow 1/2, basis, min/max, insufficient width, gap accounting, deterministic remainder, single child and tiny width.
- [x] Implement minimal allocator honoring constraints and reporting minimum-fit status.
- [x] Fix deterministic fractional remainder distribution caught by the grow 1/2 test.
- [x] Run layout tests and keep all existing box-model behavior GREEN.

### Task 4: Add renderer presets and responsive tree composition

**Files:**
- Create: `adapter/bubbletea/preset.go`
- Create: `adapter/bubbletea/render_state.go`
- Create: `adapter/bubbletea/render_helpers.go`
- Modify: `adapter/bubbletea/renderer.go`
- Modify: `adapter/bubbletea/styles.go`
- Modify/Test: `adapter/bubbletea/adapter_test.go`

**Interfaces:**
- Produces: `Preset`, `ParsePreset`, `NewRendererWithPreset`, `RenderState`, and `RenderTreeWithState` while preserving existing constructors/methods.

- [x] Add RED test proving one Document renders distinctly under minimal/dashboard/dense while preserving semantic text.
- [x] Add RED wide/narrow responsive row test.
- [x] Implement preset profiles without introducing protocol preset props.
- [x] Integrate `layout.AllocateRow` and responsive stack presentation.
- [x] Preserve equal-width legacy rows when no explicit flex object exists.
- [x] Make column composition consume child natural heights/gaps.

### Task 5: Implement semantic component rendering

**Files:**
- Create: `adapter/bubbletea/render_components.go`
- Create/Modify: `adapter/bubbletea/render_helpers.go`
- Modify/Test: `adapter/bubbletea/adapter_test.go`

**Interfaces:**
- Consumes: normalized protocol props, preset profile, immutable runtime snapshots and `RenderState`.
- Produces: terminal-bounded text, input, actions, progress, table and viewport frames.

- [x] Implement text variants and controlled style override.
- [x] Implement input focus/cursor presentation with narrow fallback.
- [x] Implement inline/toolbar/list actions with deterministic wrapping.
- [x] Implement progress bar/compact/spinner and normal/loading/success/error state rendering.
- [x] Implement table preferred widths, deterministic shrinking and record-style narrow fallback.
- [x] Implement viewport child/text rendering, clipping and follow-tail.
- [x] Add viewport clipping/follow-tail and spinner deterministic-phase tests.

### Task 6: Add one event-driven animation scheduler and prove publication isolation

**Files:**
- Modify: `adapter/bubbletea/model.go`
- Modify/Test: `adapter/bubbletea/adapter_test.go`

**Interfaces:**
- Produces: renderer-local `AnimationTickMsg` epoch/phase behavior and `NewModelWithPreset` while preserving `NewModel`.

- [x] Add RED test showing static Document needs no animation.
- [x] Add RED test showing loading spinner phase changes frame deterministically.
- [x] Add RED test showing animation tick does not advance Document revision or publish count.
- [x] Implement a single epoch-protected tick chain for the whole model.
- [x] Stop animation when no semantic node/focused input requires motion.
- [x] Keep `View()` publication conditional solely on `Engine.NeedsPublish()`.

### Task 7: Add canonical Renderer V2 showcase

**Files:**
- Create: `cmd/a2ui-runner/showcase.go`
- Modify: `cmd/a2ui-runner/main.go`

**Interfaces:**
- Produces: `-preset minimal|dashboard|dense` and `-scenario showcase` using one semantic Document.

- [x] Build one showcase Document with title/subtitle, responsive cards, progress states, table, toolbar and follow-tail viewport.
- [x] Add strict CLI preset parsing and dashboard showcase default.
- [x] Route TUI and non-TUI showcase rendering through the same engine/document semantics.
- [x] Type-check the complete CLI path in the local dependency harness.

### Task 8: Synchronize documentation and release gates

**Files:**
- Modify: `README.md`
- Modify: `SKILL.md`
- Modify: `references/PROTOCOL.md`
- Modify: `references/IMPLEMENTATION.md`
- Modify: `references/MIGRATION.md`
- Modify: `references/REVIEW-FIXES.md`
- Create: `docs/superpowers/specs/2026-09-05-a2ui-renderer-v2-design.md`
- Create: `docs/superpowers/plans/2026-09-05-a2ui-renderer-v2.md`

**Interfaces:**
- Produces: synchronized normative/agent/implementation guidance and auditable verification record.

- [x] Document box+viewport container law and new semantic presentation vocabulary.
- [x] Document preset-vs-variant and Document-vs-animation authority boundaries.
- [x] Document responsive layout and publication isolation.
- [x] Run `gofmt`/`git diff --check` and repository contract scans.
- [x] Run the maximum executable native/pure-Go, stubbed dependency, vet, race, schema and fuzz gates available in this sandbox.

### Verification environment note

The repository deliberately remains `go 1.24.0`. This sandbox contains Go 1.23.2 and cannot reach `proxy.golang.org`, so the literal repository-level Go 1.24 dependency gate cannot execute here. Verification therefore used two temporary, non-repository harnesses: a Go 1.23.2 copy for the stdlib-only semantic core, and a complete copy with API-compatible Bubble Tea/Lip Gloss stubs for compile/behavior/vet/race coverage. The repository `go.mod` and dependency declarations were never downgraded.
