# A2UI Adaptive Data Surfaces R1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add deterministic adaptive table presentation with full/compressed/master-detail/records modes and bounded vertical row windowing without changing A2UI Protocol V1 semantics.

**Architecture:** Extract renderer-only table planning into a pure `TableLayoutPlanner`. Keep semantic row selection in runtime state and keep table viewport offset in Bubble Tea `InteractionState`; terminal drawing consumes the plan and never mutates the Document or publication state.

**Tech Stack:** Go, Bubble Tea, Lip Gloss, existing A2UI Document/runtime/renderer packages.

**Spec:** `docs/superpowers/specs/2026-09-06-a2ui-adaptive-data-surfaces-r1-design.md`

## Global Constraints

- Base from current authoritative `develop`; branch remains stackable until V1 freeze is published.
- Do not change `protocol.Version`.
- Do not add/remove/rename V1 table properties or column fields.
- Keep strict unknown-property rejection unchanged.
- Presentation-only resize/mode/viewport changes must not mutate Document/runtime semantic state or publication generation.
- Use TDD: failing behavior test first, verify RED in CI, then production code.

---

### Task 1: Characterize adaptive table planner API

**Files:**
- Create: `adapter/bubbletea/table_layout_test.go`
- Create later in GREEN: `adapter/bubbletea/table_layout.go`

**Interfaces:**
- Produces renderer-only `TablePresentationMode`, `TableLayoutInput`, `TableLayoutPlan`, and `planTableLayout(TableLayoutInput) TableLayoutPlan`.

- [ ] **Step 1: Write failing tests** covering preferred-fit -> `TableModeFull`, bounded shrink -> `TableModeCompressed`, selectable four-column medium geometry -> `TableModeMasterDetail`, and too-narrow geometry -> `TableModeRecords`.
- [ ] **Step 2: Push only tests and open a draft PR.**
- [ ] **Step 3: Verify GitHub Actions fails because the planner symbols are missing.**
- [ ] **Step 4: Implement the smallest pure planner that makes those tests pass.** Use named constants for minimum column and pane widths and deterministic shrink of the widest shrinkable column.
- [ ] **Step 5: Verify full CI GREEN.**
- [ ] **Step 6: Commit production planner.**

### Task 2: Render through the planner and preserve all fields

**Files:**
- Modify: `adapter/bubbletea/render_components.go`
- Optionally create: `adapter/bubbletea/table_render_test.go`

**Interfaces:**
- Consumes `planTableLayout` from Task 1.
- Produces rendering for full/compressed/master-detail/records modes.

- [ ] **Step 1: Write failing renderer tests** using a selectable four-column table. Assert wide output contains all column headers, medium output contains a master pane plus selected-row detail values, and narrow output contains record labels for every field.
- [ ] **Step 2: Verify RED in CI before modifying production rendering.**
- [ ] **Step 3: Refactor `renderTable` to ask the planner for a plan, then dispatch to focused drawing helpers.** Keep `renderTableRecords` as the narrow all-fields fallback. Add master/detail drawing without semantic mutations.
- [ ] **Step 4: Verify focused tests and full CI GREEN.**

### Task 3: Add renderer-local table viewport state

**Files:**
- Modify: `adapter/bubbletea/interaction_state.go`
- Modify: `adapter/bubbletea/controller.go` or `adapter/bubbletea/model.go` only where key/resize synchronization requires it.
- Test: `adapter/bubbletea/adapter_test.go` or a focused new table interaction test file.

**Interfaces:**
- Produces `TableViewportState{Offset int}` stored under table node ID in `InteractionState.TableViewports`.
- Semantic selection continues to come from `runtime.TableSelection`.

- [ ] **Step 1: Write failing tests** proving selection inside the visible window does not move offset, crossing the lower boundary advances only enough to reveal the selected row, crossing the upper boundary rewinds only enough, and resize clamps an existing offset.
- [ ] **Step 2: Verify RED in CI.**
- [ ] **Step 3: Implement renderer-local table viewport state and deterministic `visibleRowWindow` calculation.**
- [ ] **Step 4: Integrate visible row slice into table/master/record rendering.**
- [ ] **Step 5: Verify GREEN including large-row tests.**

### Task 4: Prove resize is semantics-neutral

**Files:**
- Test: `adapter/bubbletea/adapter_test.go`

**Interfaces:**
- Uses existing Engine revision/publication APIs and selected row identity.

- [ ] **Step 1: Write a failing end-to-end test** that renders one immutable table at widths 120 -> 70 -> 40 -> 120 and asserts presentation changes while selected row identity, Document revision, semantic values, `NeedsPublish()` and event broker state do not change.
- [ ] **Step 2: Verify RED if any presentation path mutates authority; otherwise keep the characterization test as a regression proof and explain why no production change is needed.**
- [ ] **Step 3: Fix only the authority leak if RED exposed one.**
- [ ] **Step 4: Verify full CI GREEN.**

### Task 5: Add large-row bounded rendering proof

**Files:**
- Test: `adapter/bubbletea/table_layout_test.go`
- Test or benchmark: `adapter/bubbletea/table_render_test.go`

**Interfaces:**
- `TableLayoutPlan.RowStart` / `RowEnd` bound the rows handed to visual rendering.

- [ ] **Step 1: Add tests for 0, 1, few, 100, 1000 and 4096 rows at a fixed visible height.** Assert row window stays within the visible budget and selected row remains visible at first/middle/last positions.
- [ ] **Step 2: Verify any new failing edge case before changing production code.**
- [ ] **Step 3: Fix planner/rendering edge cases minimally.**
- [ ] **Step 4: Verify GREEN.**

### Task 6: Showcase and documentation

**Files:**
- Modify: `cmd/a2ui-runner/showcase.go`
- Modify: `docs/protocol-guide.md`
- Modify: `docs/superpowers/specs/2026-09-05-a2ui-renderer-v2-design.md`

**Interfaces:**
- Adds one canonical multi-agent activity table to the existing showcase.

- [ ] **Step 1: Add a multi-agent selectable table with columns `Agent`, `State`, `Attention`, `Task`, `Age`, `Session`, `Last event` and enough rows to demonstrate windowing.**
- [ ] **Step 2: Document that adaptive table presentation is renderer behavior, `columns[].width` is preferred width, source order is the V1 deterministic fallback importance order, and no V1 wire fields changed.**
- [ ] **Step 3: Run formatting and full verification in CI.**

### Task 7: Release verification

- [ ] **Step 1: Ensure branch diff contains no changes under `protocol/` or `assets/schema.json`.**
- [ ] **Step 2: Require `gofmt`, `go test ./...`, `go vet ./...`, `go test -race ./...`, wire fuzz smoke, document fuzz smoke, IPC fuzz smoke and conformance suite to pass.**
- [ ] **Step 3: Review PR diff for hidden semantic changes, especially selection ownership, publication barriers and event emission.**
- [ ] **Step 4: Treat GitHub required checks attached to the current PR HEAD as authoritative. Do not maintain a mutable branch SHA or workflow-run pin in PR prose; any new commit must naturally require fresh current-head checks before Ready/Merge.**
