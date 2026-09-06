# A2UI Adaptive Data Surfaces R1 Design

## Status

Approved for implementation as a renderer-only checkpoint stacked from current `develop` until A2UI Protocol V1 freeze is published. The branch must remain wire-compatible and must not change `protocol.Version`, V1 table props, or strict schema behavior.

## Goal

Make the existing Bubble Tea table presentation adapt deterministically to terminal geometry and row cardinality while preserving the authoritative A2UI Document and runtime semantics.

One semantic V1 table should render as:

```text
wide        -> full table
medium      -> compressed table while every column stays readable
medium      -> master/detail when tabular compression would violate readability
narrow      -> record view
many rows   -> vertically windowed presentation
```

## Authority law

```text
Agent operations
  -> Document
  -> Runtime semantic projection
  -> TableLayoutPlanner
  -> Bubble Tea local presentation state
  -> terminal frame
```

Terminal resize, table presentation mode, row-window offset and pane geometry are presentation-only. They must not mutate the Document, advance revision/publication generation, emit semantic events, change selected row identity, or change runtime focus order.

Fundamental focus rule:

```text
adaptive presentation != semantic tree
```

A master/detail detail pane is a display projection of the selected row. It is not a `document.Node`, is not focusable, never enters `runtime.FocusableIDs`, and cannot alter Tab order. Row navigation remains semantic table selection on the original table node in every presentation mode. A terminal resize therefore cannot add/remove focus targets.

## V1 freeze boundary

R1 does not add or reinterpret wire fields. Existing V1 table input remains:

```text
columns: [{title, width?}]
rows: [[string...]]
selectable
row_ids
action
variant
flex
```

`columns[].width` remains a preferred presentation width. Source column order is the only deterministic importance ordering available in V1; R1 must not infer semantics from titles or values.

Future fields such as `priority`, `kind`, `min_width`, `max_width`, `grow` and `collapse` are explicitly outside this checkpoint.

## Presentation modes

Renderer-only modes:

```go
type TablePresentationMode int

const (
    TableModeFull TablePresentationMode = iota
    TableModeCompressed
    TableModeMasterDetail
    TableModeRecords
)
```

Mode selection is a pure function of geometry, variant/preset, preferred column widths, current sanitized dataset, column count, row count and selectability.

There is no terminal breakpoint such as `width < 80`. Mode switches are consequences of whether the current table can satisfy its intrinsic presentation constraints inside available geometry.

### Full

Preferred widths fit in available width.

### Compressed

Preferred widths do not fit, but every column can remain at or above its computed readable floor.

For R1, each readable floor is derived deterministically from existing V1 information:

- hard renderer minimum;
- a conservative fraction of `columns[].width` (the existing preferred width hint);
- observed sanitized title/cell width, capped by the preferred width.

This is a renderer policy, not a new protocol semantic. No column may be compressed below its computed floor merely because a particular terminal width was crossed.

### Master/detail

Eligible only when the table is selectable, non-empty, has at least four columns and there is enough width for two useful panes derived from the current readable floors.

The master pane shows the longest source-order prefix that fits without violating readable floors. The detail pane shows every field of the selected row, so no semantic field is silently discarded.

The detail pane is pure rendering output. It has no independent focus, selection, key routing, scroll state or action authority.

### Records

Used when a useful tabular/master-detail presentation cannot fit. Every row is rendered as `Column: value` records, preserving all fields.

## Table layout planner

Table geometry policy lives in `adapter/bubbletea/table_layout.go`.

The planner receives available width/height, full sanitized columns and rows, row count, selectability, semantic selection, current row-window cache and table style inputs. It returns a deterministic `TableLayoutPlan` containing mode, preferred/resolved/readable widths, visible master columns, pane widths and visible row range.

The planner is a permanent layer boundary. The same five-column agent-activity dataset has paired discriminator tests:

```text
width 80  -> MasterDetail
width 120 -> Full
```

These tests distinguish cost-model failures from renderer/integration failures without print-debugging.

### Single-plan integration law

The renderer must compute the table plan exactly once from the complete sanitized dataset before vertical row slicing:

```text
full dataset
  -> PlanTableLayout
  -> mode + column/pane geometry + row range
  -> slice visible rows
  -> draw according to that plan
```

A projected/windowed row slice must not be passed back through a second planner call. Otherwise row windowing can silently change the selected presentation mode and bypass dataset-derived readability decisions.

## Vertical windowing

Large tables render only rows visible in the current terminal height. The selected row must always remain visible.

Renderer-local state contains:

```go
type TableViewportState struct {
    Offset int
}
```

Despite the historical name, this is **not** a second user-controlled scroll state. Semantic selection remains runtime-owned. `Offset` is only a derived/cache value for the top visible row so adjacent semantic selection moves can avoid needless jumps.

Derived-window law:

```text
semantic selection inside window -> cached top row unchanged
selection below window           -> advance minimum necessary amount
selection above window           -> rewind minimum necessary amount
resize / data change             -> clamp/recompute from selection + geometry
stable row-id reorder            -> runtime follows row ID; cache follows resulting index
removed/non-selectable table     -> prune cache
```

There are no independent table `PageUp/PageDown` scroll commands and no table `PinnedToTail` state.

This intentionally differs from a scrollable `viewport`:

```text
viewport: user directly owns presentation scroll offset + follow-tail pin
 table:   user owns semantic row selection; top-row offset is derived from it
```

Because the state machines and authorities differ, R1 does not force them into one generic scroll primitive. Shared low-level clamp helpers are acceptable, but ownership semantics remain distinct.

## Resize law

For the same Document and runtime state, resize may change only presentation. A wide -> narrow -> wide cycle must preserve selected row identity, Document revision, semantic values, session, publication state and runtime focus order.

Focus regression coverage must force Full, MasterDetail and Records modes for the same Document and prove the same deterministic Tab cycle in every geometry.

## Presets

`minimal`, `dashboard` and `dense` may change borders, spacing and decoration. They must not change semantic selection, row identity, actions, focus order or data presence. Mode selection stays geometry/data-driven rather than preset-breakpoint-driven.

## Non-goals

R1 does not add sorting, filtering, search, horizontal scrolling, independent table scrolling, pinned columns, cell editing, typed cells, mouse support, lazy server data, pagination protocol, Web rendering or protocol semantic hints.

## Acceptance

A single immutable selectable table must demonstrate full, compressed, master/detail and record presentation across terminal widths, restore its original presentation on resize back, preserve selected `row_id`, preserve deterministic runtime focus order, and emit no semantic/publication side effects from presentation-only changes.

Large row sets up to the V1 limit must render a bounded visible row window rather than constructing the full visual table each frame. That window must follow runtime-owned semantic selection and remain a renderer-local derived cache rather than becoming a second scroll authority.
