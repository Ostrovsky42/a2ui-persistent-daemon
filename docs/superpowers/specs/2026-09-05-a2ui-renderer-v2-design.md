# A2UI Renderer V2 — Dynamic Semantic Presentation Design

## Status

Approved and implemented on the Renderer V2 feature branch. This design records the architectural boundary used by the implementation.

## Goal

Make the existing Bubble Tea adapter expressive, responsive and dynamic while preserving A2UI's hardened authority and publication laws. One semantic Document must support three renderer-side presets: `minimal`, `dashboard`, and `dense`.

## Authority model

```text
Agent operations
  -> Document (authoritative semantic state)
  -> Runtime projection (focus, input values, bindings, publication generation)
  -> Semantic presentation resolution
  -> Bubble Tea ephemeral state (terminal size, animation phase, cursor phase, selection)
  -> Preset/theme
  -> terminal frame
```

Renderer state is never a second Document. Animation frames, wall-clock time, cursor blink and terminal caches are forbidden from authoritative state.

## Container correction

The existing adapter already traversed `viewport.Children`, while protocol validation allowed only `box` to parent children. Renderer V2 resolves that contradiction at the semantic owner: `box` and `viewport` are containers; `text`, `table`, `input`, `actions`, and `progress` are leaves.

Viewport remains both a container and a retained-text append target.

## Semantic presentation vocabulary

The protocol exposes bounded intent rather than terminal styling implementation details:

- `text.variant`: `body|title|subtitle|label|code|muted`
- `box.variant`: `plain|panel|card|section`
- `actions.variant`: `inline|toolbar|list`
- `progress.variant`: `bar|compact|spinner`
- `progress.state`: `normal|loading|success|error`
- `table.variant`: `normal|compact|dense`
- `box.align`: `start|center|end|stretch`
- `box.responsive`: `none|stack`
- `viewport.follow_tail`: boolean
- common `flex`: `grow`, `basis`, `min_width`, `max_width`

All fields remain strict. Invalid enums, negative flex values, contradictory width limits and unknown nested fields reject the mutation before authoritative state changes.

Presentation defaults obey `explicit protocol property > variant/preset default > global preset default`. Because normalization materializes defaults, the Document retains top-level explicit-property presence as semantic metadata. A full upsert replaces that presence set; props patches add supplied keys; input `force` is excluded because it is one-shot policy.

The protocol does not expose renderer preset, animation phase, raw ANSI, arbitrary RGB, CSS cascade, percentage sizing or absolute positioning.

## Presets

Preset is renderer/application configuration, not an A2UI property.

### Minimal

Quiet tool-console presentation: low border density, restrained accents and normal spacing.

### Dashboard

Stronger cards/panels, hierarchy, accents and status presentation suitable for control-plane/HUD views.

### Dense

Tight spacing, compact controls/tables and reduced visual overhead for developer/admin terminals.

Given identical Document, runtime state, terminal dimensions and animation state, rendering is deterministic. No random style selection exists.

## Layout

`layout.AllocateRow` is renderer-neutral and deterministic. It accepts each child's basis, grow weight, minimum and maximum widths, and accounts for parent gaps.

When declared minimum widths fit, the allocator honors them and distributes remaining width by grow weight with deterministic remainder assignment. When minimum widths cannot fit, the allocator reports that condition.

A `row` with `responsive:"stack"` then renders visually as a column. This does not reparent nodes or mutate the Document. A non-responsive row uses deterministic constrained allocation.

Legacy rows without an explicit `flex` property retain equal-share behavior.

Column composition consumes child natural rendered height and gaps instead of giving every child the full terminal height budget.

## Component rendering

Text variants resolve semantic hierarchy through the active preset before controlled explicit style overrides.

Actions support inline, toolbar and list composition, with deterministic narrow wrapping.

Progress supports determinate bar, compact and spinner forms plus semantic normal/loading/success/error states. Loading phases are renderer-local.

Tables first try preferred column widths, shrink deterministically to bounded minimums, and use a record-oriented narrow fallback when tabular columns cannot fit. Rightmost columns are not silently discarded.

Viewport clips to `height`; `follow_tail:false` selects the head and `follow_tail:true` selects the tail. Manual scrolling is intentionally deferred.

## Animation scheduler

There is one Bubble Tea scheduler per Model, not one timer per component. A static Document schedules no animation tick.

An active spinner/loading bar or focused input can activate the scheduler. Tick messages carry an epoch so stale chains are ignored after animation stops. A tick changes only renderer-local phase/cursor visibility and schedules another tick only while animation remains active.

## Publication isolation

Commit publication remains:

```text
accepted commit
  -> Engine.NeedsPublish() == true
  -> frame is rendered
  -> Engine.Publish()
  -> committed event
```

Animation-only redraws do not mutate Document/runtime projection, do not advance Document revision, do not make `NeedsPublish()` true and do not emit `committed`.

## CLI showcase

`cmd/a2ui-runner` accepts `-preset minimal|dashboard|dense` and a `showcase` scenario. The showcase constructs one canonical Document containing semantic text hierarchy, responsive cards, progress states, a table, toolbar actions and a follow-tail viewport. The same Document is used for all presets.

## Compatibility

Existing constructors remain available and default to the conservative minimal preset. Additive constructors select a preset explicitly. Legacy A2UI documents remain valid and receive deterministic default variants.

## Non-goals

Renderer V2 does not add Web rendering, mouse handling, manual viewport scroll state, drag/drop, absolute or percentage positioning, modals/window management, an animation DSL, easing curves, arbitrary agent-supplied timing, raw ANSI, arbitrary RGB, random visual generation or theme negotiation over MCP.

## Verification strategy

Tests prove protocol strictness, viewport container invariants, flex allocator behavior, preset differentiation on one Document, responsive wide/narrow behavior, viewport clipping/follow-tail, deterministic animation phase, idle scheduler behavior and publication isolation.

Release verification additionally requires formatting, full tests, vet, race tests and existing wire/document fuzz smoke when the required Go toolchain and dependencies are available.
