# A2UI v1 Hardened — normative protocol

`PROTOCOL.md` is normative. `SKILL.md` is a shortened agent-facing projection. If they disagree, this document wins.

Normative terms **MUST**, **MUST NOT**, **SHOULD**, **SHOULD NOT**, and **MAY** are used in their ordinary RFC-style sense.

## 1. Scope

A2UI synchronizes a declarative UI document from an untrusted or semi-trusted agent to a trusted local runtime. The protocol does not permit arbitrary code execution. A renderer (Bubble Tea, web, SDL, embedded display) is an adapter and MUST NOT become the source of truth for protocol state.

A2UI v1 keeps six document operations:

`upsert`, `props`, `text`, `remove`, `focus`, `commit`.

Node catalog:

`box`, `text`, `viewport`, `table`, `input`, `actions`, `progress`.

## 2. Layers and authority

A conforming implementation separates:

```text
transport -> wire -> session -> protocol -> document/reconciler -> runtime projection -> renderer
```

`document.Document` semantics are authoritative agent-owned state. A2UI v1 distinguishes renderer-independent runtime projection from adapter-local presentation mechanics:

```text
Document            table rows, row IDs, viewport policies, props, tree
Runtime projection  focused node, input value, semantic table selection
Adapter local       input caret, viewport offset/tail pin, animation phase, terminal size
```

Only state that changes the meaning of a subsequent semantic user action belongs in Runtime projection. Purely visual navigation mechanics remain adapter-local. A runtime MUST NOT determine whether a node exists by looking only in a widget pool.

## 3. Transaction rule

Every state-changing operation follows:

```text
current Document
    + operation
    -> candidate
    -> schema/semantic validation
    -> structural/resource invariant validation
    -> commit candidate as revision N+1
```

If any validation step fails, the authoritative Document MUST remain unchanged.

No partial props write, partial tree mutation, or poisoned error component is allowed.

## 4. Wire representations

All JSON records carry `"v":1`.

### 4.1 Legacy record

For migration/debugging, the runtime accepts the original operation form directly:

```json
{"v":1,"seq":1,"op":"commit","frame":"boot"}
```

### 4.2 Hardened envelope

Reliable and multiplexed profiles SHOULD use:

```json
{
  "v": 1,
  "session": "ui-42",
  "kind": "operation",
  "seq": 17,
  "payload": {"v":1,"seq":17,"op":"commit"}
}
```

`kind` is one of:

- `hello`
- `hello_ack`
- `operation`
- `event`
- `telemetry`

Unknown top-level wire fields MUST be rejected in v1. Duplicate JSON object keys MUST be rejected at any nesting depth. This avoids differential-parser ambiguity.

## 5. Session negotiation

Reliable hardened profiles use an A2UI session state machine:

```text
NEW -> READY -> DRAINING -> CLOSED
```

A `hello` payload advertises supported protocol versions and optional features. The runtime selects one version and returns `hello_ack` containing:

- selected version;
- supported node types;
- accepted feature set;
- finite resource limits.

If version intersection is empty, the error is fatal and the session does not become READY.

A2UI session state is application state. When carried over modern MCP, it MUST be represented explicitly by the A2UI session handle; implementations MUST NOT assume an MCP transport session exists.

## 6. Sequence numbers and revisions

Three concepts are independent:

1. reliable inbound mutation sequence;
2. outbound event sequence;
3. Document revision.

For hardened reliable sessions, mutation sequence starts at 1 and increments by exactly one.

- `seq == expected`: accept for processing;
- `seq < expected`: duplicate; MUST NOT apply twice;
- `seq > expected`: `protocol.sequence_gap`, fatal for that reliable session.

Document revision increments only after an accepted state-changing mutation. `focus` and `commit` do not by themselves change Document revision.

Legacy direct operation `seq:0` remains legal for compatibility outside hardened reliable sequencing.

## 7. `upsert`

```json
{"v":1,"seq":1,"op":"upsert","id":"log","type":"viewport","parent":"main","index":0,"props":{"height":10}}
```

Creation:

- `id` and `type` are required;
- default parent is `root`;
- parent MUST exist;
- parent MUST be a container (`box` or `viewport`);
- index outside valid insertion range appends;
- candidate MUST remain within node/depth/children limits.

Existing node:

- props are a **full replacement**;
- omitted properties take type defaults;
- same-type runtime-local state SHOULD be preserved;
- `parent` change is rejected as `document.reparent_unsupported`;
- type change is legal only if structural invariants remain valid and runtime-local state for that node is recreated.

Invalid upsert MUST NOT reserve/poison the ID. A later valid upsert with the same ID can succeed.

## 8. `props`

```json
{"v":1,"seq":2,"op":"props","id":"main","props":{"gap":2}}
```

`props` performs a **shallow top-level merge** against the current normalized props for that node type.

Nested objects are replaced, not recursively merged. For example replacing `style` replaces the complete style object supplied in the patch.

The merge is transactional: merge into a candidate map, validate the complete result, then commit it.

`props` works for containers as well as leaf widgets because node existence belongs to the Document.

## 9. `text`

```json
{"v":1,"seq":3,"op":"text","id":"log","text":"line\n"}
```

Only `text` and `viewport` accept append operations. Other types return `document.text_unsupported`.

Retained text is authoritative document data and counts against both per-node and total retained-text budgets.

`viewport` is a container and a retained-text target. `height` bounds the visible line window. `follow_tail` declares tail-follow policy. When `scrollable:true`, a local adapter MAY expose manual scrolling; the current offset and tail-pin state MUST remain adapter-local and MUST NOT mutate the Document.

## 10. `remove`

`remove` deletes a node and its complete subtree. `root` is immutable.

Removal updates retained-resource accounting transactionally. Runtime projection removes local state belonging to removed IDs.

If focus was inside the removed subtree, focus is reconciled deterministically to the first remaining focusable node in tree preorder. If none remains, focus becomes empty.

## 11. `focus`

Focus target MUST exist and be focusable:

- `input` is focusable;
- `table` is focusable only when `selectable:true`;
- `viewport` is focusable only when `scrollable:true`.

Focus is runtime-local state and exactly one node or none is focused.

If a focused node later becomes non-focusable, is removed, or changes type, runtime MUST reconcile focus deterministically; stale focus is invalid.

## 12. `commit`

`commit` is a **publication barrier**, not a transaction around earlier mutations.

Earlier accepted mutations are already authoritative. `commit` requires the renderer/runtime publication path to expose at least the current Document revision and yields a reliable event:

```json
{"v":1,"ev":"committed","seq":12,"row":0,"revision":8,"through_seq":17,"frame":"boot"}
```

`through_seq` identifies the mutation stream boundary represented by the publication acknowledgement.

A renderer MAY coalesce ordinary dirty revisions before publication. It MUST NOT need a permanent 60 Hz tick when there is no dirty work.

## 13. Node props

### Common presentation hints

Presentation hints are semantic and renderer-neutral. They do not encode a concrete theme. A renderer preset such as `minimal`, `dashboard`, or `dense` is local renderer configuration and MUST NOT be sent as an A2UI node property.

Presentation resolution uses this precedence when a semantic variant supplies renderer defaults:

```text
explicit protocol property > variant/preset default > global preset default
```

The reconciled Document therefore retains which top-level properties were explicitly supplied. That presence metadata is authoritative presentation semantics, is bounded by the Document resource budget, and is reset by full `upsert` replacement. One-shot mutation policy such as input `force` is never retained as explicit presentation metadata.

All renderable node types MAY carry a strict `flex` object:

```json
{"flex":{"grow":1,"basis":30,"min_width":20,"max_width":80}}
```

Allowed members are only `grow`, `basis`, `min_width`, and `max_width`. `grow` and `basis` MUST be non-negative integers. Minimum/maximum widths MUST be positive integers, and when both are present `max_width >= min_width`. Unknown members reject the mutation.

Flex hints describe allocation inside a parent `row`. They MUST NOT change the Document tree. A renderer MAY deterministically constrain allocation when space is insufficient.

### `box`

Defaults:

```json
{"dir":"col","padding":0,"gap":0,"border":"none","style":{},"variant":"plain","align":"start","responsive":"none"}
```

`box` and `viewport` are the only container node types.

`dir`: `col|row`.
`border`: `none|rounded|normal`.
`variant`: `plain|panel|card|section`.
`align`: `start|center|end|stretch`.
`responsive`: `none|stack`.
`padding` and `gap` are non-negative integers.

Border and padding consume layout width/height. Gap applies in both row and column directions. `responsive:"stack"` permits a row to render visually as a column when declared minimum child widths cannot fit; this is a presentation transform and MUST NOT mutate the Document.

### `text`

```json
{"text":"","style":{},"variant":"body"}
```

`variant`: `body|title|subtitle|label|code|muted`.

### `viewport`

```json
{"height":10,"wrap":true,"follow_tail":false,"scrollable":false}
```

`viewport` may contain child nodes and also accepts retained text append operations. Renderer clamps height to available terminal space. `scrollable:true` makes the viewport focusable and permits adapter-local manual scrolling. `follow_tail:true` means a viewport that is currently pinned to tail follows appended/wrapped content. Manual scrolling above the tail MUST unpin without changing Document state; returning to the exact tail MAY repin, and `End` SHOULD repin in terminal adapters. Offsets operate on rendered visual lines after wrapping and MUST be clamped after content or terminal-width changes.

### `table`

```json
{"columns":[{"title":"Device","width":16}],"rows":[["bus-1"]],"row_ids":["device-bus-1"],"selectable":true,"action":"open_device","variant":"normal"}
```

`variant`: `normal|compact|dense`. Column title is required. If width is present it MUST be >= 1. Row/column counts are bounded by negotiated limits.

`row_ids` is optional. When present it MUST contain exactly one non-empty unique string per row. Runtime selection follows the stable `row_id` when rows reorder; without row IDs it preserves/clamps the numeric index. If the selected stable row disappears, selection clamps to the nearest valid row. Empty tables have no selected row. Selection is Runtime projection state and MUST NOT change Document revision or require protocol publication acknowledgement.

`action` is an optional semantic activation ID. Activating a selected row emits a critical `select` event containing table `id`, numeric `row`, optional `row_id`, and `action`. Arrow/Home/End selection movement itself does not emit wire events.

A terminal renderer MUST respect its available width; when columns cannot fit, deterministic shrinking or a record-style narrow fallback is preferred over silently dropping rightmost columns.

### `input`

Persistent props:

```json
{"placeholder":"","value":"","action":""}
```

For wire compatibility an input mutation MAY include `"force":true`. `force` is a one-shot mutation policy and MUST NOT be retained in Document props.

Rules:

- on creation, `value` initializes runtime-local input value;
- later mutations do not overwrite user-edited local value unless the same mutation has `force:true`;
- a later unrelated mutation MUST NOT repeat an earlier force.

### `actions`

```json
{"items":[{"key":"r","label":"Reload","action":"dsp.reset","args":{}}],"variant":"inline"}
```

`variant`: `inline|toolbar|list`. Key is exactly one Unicode rune. Nested unknown fields are rejected.

Bindings are indexed deterministically in tree preorder. A duplicate key at the same v1 scope rejects the candidate with `runtime.binding_conflict`; runtime MUST NOT choose a winner by Go map iteration or other nondeterministic ordering.

### `progress`

```json
{"value":0.42,"label":"Sync","variant":"bar","state":"normal"}
```

`variant`: `bar|compact|spinner`.
`state`: `normal|loading|success|error`.

When present, value MUST be in `[0,1]`. Invalid values reject the mutation. Renderer-local animation phase for loading/spinner presentation MUST NOT be stored in Document or advance Document revision.

### `style`

```json
{"fg":"primary","bg":"muted","bold":false,"dim":false}
```

Allowed color tokens: `primary`, `warn`, `error`, `muted`.

Arbitrary hex/rgb values are rejected as `schema.invalid_color`. Theme token mapping belongs to the renderer/runtime. Semantic state may map to additional renderer-local theme roles such as success/focus without expanding the wire color vocabulary.

## 14. Renderer presets and animation

A renderer MAY expose local presets such as `minimal`, `dashboard`, and `dense`. Presets choose concrete borders, spacing, glyphs and terminal theme roles for the same semantic Document. Preset selection is outside the A2UI protocol.

Animation is renderer-local ephemeral state. An idle renderer MUST NOT require a perpetual timer. When a semantic state requires motion, a renderer MAY schedule bounded event-driven ticks and update only local animation phase/cursor phase. Such ticks MUST NOT:

- change Document revision;
- change mutation sequence;
- synthesize `commit`;
- emit `committed`;
- call publication merely because the animation frame changed.

The existing publication barrier remains authoritative: accepted mutations become visible, then `Engine.Publish()` acknowledges pending commit barriers.

## 15. Interactive runtime and user events

A host MAY map local keys to semantic interaction. Terminal adapters SHOULD use deterministic routing:

```text
process control -> focus traversal -> focused component -> global action binding -> ignore
```

Thus a rune typed into a focused input MUST NOT also execute a global action bound to that rune. Focus traversal remains deterministic tree preorder.

Input value is Runtime projection state; caret position is adapter-local. Rune insertion/deletion MUST NOT split UTF-8. Local input editing, table selection movement, viewport scrolling, caret movement and animation redraw MUST NOT synthesize `commit` or `committed`.

A renderer SHOULD consume one coherent presentation snapshot. The reference Engine exposes an atomic snapshot containing cloned Document, focus, input values and table selections so concurrent agent mutation cannot produce a mixed-revision frame.

`select` is a critical/reliable event. Example:

```json
{"v":1,"seq":42,"ev":"select","id":"jobs","row":2,"row_id":"job-105","action":"open_job"}
```

Critical event backpressure MUST be explicit (`runtime.backpressure_exceeded`); selection activation MUST NOT be silently dropped.
