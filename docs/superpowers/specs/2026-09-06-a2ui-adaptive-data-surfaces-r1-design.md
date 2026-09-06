# A2UI Adaptive Data Surfaces R1 Design

## Status

Approved for implementation as a renderer-only checkpoint stacked from current `develop` until A2UI Protocol V1 freeze is published. The branch must remain wire-compatible and must not change `protocol.Version`, V1 table props, or strict schema behavior.

## Goal

Make the existing Bubble Tea table presentation adapt deterministically to terminal geometry and row cardinality while preserving the authoritative A2UI Document and runtime semantics.

One semantic V1 table should render as:

```text
wide        -> full table
medium      -> compressed table
medium      -> master/detail when selectable and useful
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

Terminal resize, table presentation mode, row-window offset and pane geometry are presentation-only. They must not mutate the Document, advance revision/publication generation, emit semantic events, or change selected row identity.

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

Mode selection is a pure function of geometry, variant/preset, preferred column widths, column count, row count and selectability.

### Full

Preferred widths fit in available width.

### Compressed

Preferred widths do not fit, but every column can remain tabular at the bounded renderer minimum width.

### Master/detail

Eligible only when the table is selectable, non-empty, has at least four columns and there is enough width for two useful panes. The master pane shows the longest source-order prefix that fits. The detail pane shows every field of the selected row, so no semantic field is silently discarded.

### Records

Used when a useful tabular/master-detail presentation cannot fit. Every row is rendered as `Column: value` records, preserving all fields.

## Table layout planner

Extract table geometry policy from terminal drawing into `adapter/bubbletea/table_layout.go`.

The planner receives available width/height, columns, row count, selectability, selection and table style inputs. It returns a deterministic `TableLayoutPlan` containing mode, resolved widths, visible master columns, pane widths and visible row range.

Policy thresholds must be named constants in one place. No scattered width magic numbers are allowed.

## Vertical windowing

Large tables render only rows visible in the current terminal height. The selected row must always remain visible.

Renderer-local state is extended with:

```go
type TableViewportState struct {
    Offset int
}
```

Semantic selection remains runtime-owned. The viewport offset answers only which slice of rows is visible.

Scrolling law:

```text
selection inside window -> offset unchanged
selection below window  -> advance minimum necessary amount
selection above window  -> rewind minimum necessary amount
```

Do not recenter after every keypress.

## Resize law

For the same Document and runtime state, resize may change only presentation. A wide -> narrow -> wide cycle must preserve selected row identity, Document revision, semantic values, session and publication state.

## Presets

`minimal`, `dashboard` and `dense` may change borders, spacing and decoration. They must not change semantic selection, row identity, actions or data presence. Mode selection should stay geometry-driven.

## Non-goals

R1 does not add sorting, filtering, search, horizontal scrolling, pinned columns, cell editing, typed cells, mouse support, lazy server data, pagination protocol, Web rendering or protocol semantic hints.

## Acceptance

A single immutable selectable table must demonstrate full, compressed, master/detail and record presentation across terminal widths, restore its original presentation on resize back, preserve selected `row_id`, and emit no semantic/publication side effects from presentation-only changes.

Large row sets up to the V1 limit must render a bounded visible row window rather than constructing the full visual table each frame.
