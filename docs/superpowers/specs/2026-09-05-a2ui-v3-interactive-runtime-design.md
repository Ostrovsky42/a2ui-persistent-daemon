# A2UI V3 — Interactive Runtime Design

## Status

Approved architecture for the V3 interactive-runtime checkpoint. Source authority remains A2UI v1 semantic protocol; V3 is additive and does not create a second protocol version.

## Goal

Turn the expressive Renderer V2 into a bidirectional terminal runtime: users can navigate focus, select table rows, manually scroll live viewports and edit Unicode input while an agent concurrently changes the authoritative Document.

## Authority model

V3 separates state by semantic ownership:

```text
Document
  agent-owned semantic state
  rows, row IDs, props, tree, scroll/follow policy

Runtime projection
  renderer-independent interaction meaning
  focused node, input value, table selection

Bubble Tea adapter-local
  presentation mechanics
  input caret, viewport offset/tail pin, animation phase, terminal size
```

A state belongs in Runtime projection only when it changes the semantic meaning of a subsequent user action. A viewport offset or caret position changes what the operator sees/edits locally but is not itself protocol state.

## Publication versus redraw

Renderer V2 used one generation concept for both local redraw and publication dirtiness. V3 splits those laws:

```text
agent semantic mutation -> redraw + publication generation
local semantic interaction -> redraw only
adapter presentation interaction -> Bubble Tea redraw only
```

`Engine.NeedsPublish()` observes only agent publication work and pending commit barriers. Local focus, input edits and table selection cannot fabricate `committed` events.

## Stable table selection

Selectable tables may provide `row_ids` and `action`. `row_ids` are unique, non-empty and exactly parallel to `rows`.

Runtime stores `TableSelection{Index, RowID}`. When rows reorder, a stable selected RowID is resolved to its new index. Without IDs the numeric index is preserved/clamped. Removing the selected row clamps to the nearest valid row; an empty/non-selectable/removed table has no selection.

Arrow/Home/End navigation changes Runtime projection locally and emits no protocol traffic. Enter activates the current row and emits one critical `select` event with table ID, numeric row, stable row ID when available, and action.

## Viewport manual scrolling

`viewport.scrollable` is a Document capability. A scrollable viewport participates in deterministic focus traversal.

Its current offset and `PinnedToTail` state remain Bubble Tea-local. Renderer returns metrics derived from actual wrapped visual lines so the controller never duplicates layout calculations. Manual scrolling above tail unpins. `End`, and downward navigation reaching the exact bottom, repins when `follow_tail:true`. Agent append while unpinned preserves reading position; append while pinned follows the new tail. Resize/content shrink recomputes/clamps metrics.

## Input caret

Input value remains Runtime projection. Caret is Bubble Tea-local and rune-indexed. Left/Right/Home/End move it; Backspace/Delete and rune insertion edit through `Engine.SetInput`; Enter emits the existing submit event. Agent `force:true` value replacement clamps the local caret to the new rune length.

## Deterministic key routing

Terminal key ownership is:

```text
process control
  -> Tab / Shift+Tab focus traversal
  -> focused component controller
  -> global action bindings
  -> ignore
```

A focused input therefore consumes typed runes before global actions. Table and viewport navigation keys likewise cannot accidentally trigger an action binding.

## Atomic rendering snapshot

`Engine.PresentationSnapshot()` clones Document, focus, input values and table selections under one Engine mutex. Bubble Tea rendering consumes that immutable snapshot instead of combining several independently locked reads.

## Renderer contract

`RenderFrame` stays stateless and receives semantic snapshot plus adapter-local `InteractionState`. It returns a `RenderResult` containing the frame and `ViewportMetrics`. Existing V2 render APIs remain compatibility wrappers.

## Dynamic E2E

The canonical interaction scenario contains a stable-ID jobs table, details card, progress, scrollable follow-tail logs, command input and action toolbar. The test drives the agent via the MCP simulator and the operator via Bubble Tea key messages, proving:

1. table selection + critical `select` event;
2. selection survival after agent row reorder;
3. manual viewport unpin during agent log append;
4. tail repin and subsequent following;
5. rune-safe caret editing + submit;
6. terminal resize with semantic interaction state preserved;
7. the same flow under minimal, dashboard and dense presets.

## Compatibility

V1/V2 documents remain valid. Tables without `row_ids` retain index-based selection. Viewports without `scrollable:true` retain passive V2 clipping/follow-tail behavior. Public V2 renderer/model constructors remain available.

## Non-goals

V3 does not add mouse/hit testing, drag/drop, modals, tabs/tree/select widgets, multiline editor, clipboard, remote per-keystroke change events, arbitrary keymaps, Web rendering, transport redesign, theme negotiation or animation DSL.
