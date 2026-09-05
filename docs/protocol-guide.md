# A2UI six-operation protocol guide

This is a practical guide to the public A2UI v1 mutation vocabulary. The normative contract remains [`references/PROTOCOL.md`](../references/PROTOCOL.md).

A2UI is not "NDJSON as a UI format." The semantic protocol is transport-independent. NDJSON is one record/debug representation; hardened reliable sessions wrap mutations in versioned envelopes.

## The six mutations

A2UI v1 has exactly six public mutation types:

```text
upsert
props
text
remove
focus
commit
```

`hello`, `hello_ack`, transport messages, daemon/client IPC records, user events, and telemetry are not additional UI mutations.

## Runnable reference fixture

The repository contains one hardened-session walkthrough:

```text
assets/examples/omarchy-choice.ndjson
```

It is replayed by:

```bash
go test ./conformance -run TestOmarchyChoiceFixtureReplaysAllSixOperations
```

The fixture intentionally exercises all six mutation types in one coherent session. It creates a choice panel, patches it, streams one log line, removes a temporary node, focuses an input, and finishes with a publication barrier.

## Reliable session shape

A reliable session starts with `hello`:

```json
{"v":1,"session":"omarchy-choice","kind":"hello","seq":0,"payload":{"versions":[1],"features":["commit-barrier"]}}
```

The runtime negotiates the version and supported features before reliable mutations begin.

Mutations then start at sequence `1` and increase by exactly one:

```json
{
  "v": 1,
  "session": "omarchy-choice",
  "kind": "operation",
  "seq": 1,
  "payload": {
    "v": 1,
    "seq": 1,
    "op": "upsert",
    "id": "choice-panel",
    "type": "box",
    "parent": "root",
    "props": {"dir":"col","gap":1}
  }
}
```

The envelope `seq` and operation `seq` must match.

For hardened reliable sequencing:

```text
seq == expected   accept
seq <  expected   duplicate; do not apply twice
seq >  expected   protocol.sequence_gap
```

Mutation sequence, outbound event sequence, and Document revision are independent counters.

---

## 1. `upsert`

Use `upsert` to create a node or to fully replace the props of an existing node.

```json
{"v":1,"seq":1,"op":"upsert","id":"panel","type":"box","parent":"root","props":{"dir":"col","gap":1}}
```

### Creation rules

- `id` and `type` are required;
- omitted parent defaults to `root`;
- parent must already exist;
- only `box` and `viewport` can contain children;
- depth, node count, child count, retained bytes, and component-specific limits still apply.

### Existing-node rules

`upsert` is a **full props replacement**.

If an existing table had:

```json
{"columns":[{"title":"Name"}],"rows":[["A"]],"selectable":true}
```

and receives an `upsert` with only:

```json
{"rows":[]}
```

omitted fields return to their defaults. Use `props` when you intend a patch.

### Typical errors

- missing parent → document validation error;
- non-container parent → `document.parent_not_container`;
- reparent attempt on existing node → `document.reparent_unsupported`;
- invalid property/color/type → candidate rejected without changing the current Document.

---

## 2. `props`

Use `props` to shallow-merge top-level properties into an existing node.

```json
{"v":1,"seq":7,"op":"props","id":"choice-panel","props":{"gap":2}}
```

The merge is **top-level only**. Nested objects are replaced as a unit.

For example:

```json
{"style":{"fg":"primary","bold":true}}
```

followed by:

```json
{"style":{"fg":"warn"}}
```

replaces the previous `style` object; it does not preserve `bold:true` by recursive merge.

The complete candidate is normalized and validated before it becomes authoritative.

### Typical errors

- unknown node;
- unknown property;
- invalid complete post-merge props;
- resource/invariant violation.

A failed patch must leave the Document unchanged.

---

## 3. `text`

Use `text` for incremental retained text output.

```json
{"v":1,"seq":8,"op":"text","id":"activity","text":"Choices loaded.\n"}
```

Only `text` and `viewport` nodes accept append operations.

This is the low-overhead streaming path: build the layout once, then append content without resending the tree.

Retained text counts against both:

- per-node retained text budget;
- total retained text budget.

### Typical errors

- target is not `text`/`viewport` → `document.text_unsupported`;
- per-node or total text budget exceeded → resource error.

A `viewport` may also contain child nodes. Its manual scroll offset remains renderer-local even though appended text is authoritative Document data.

---

## 4. `remove`

Use `remove` to delete one node and its complete subtree.

```json
{"v":1,"seq":9,"op":"remove","id":"temporary-note"}
```

`root` is immutable.

Removing a subtree also reconciles renderer-independent runtime state associated with removed IDs. If focus disappears, runtime picks the first remaining focusable node in deterministic tree preorder, or clears focus if none remain.

### Typical errors

- attempt to remove `root` → `document.root_immutable`;
- unknown/invalid target according to the current semantic contract.

---

## 5. `focus`

Use `focus` to request semantic focus without changing the Document tree.

```json
{"v":1,"seq":10,"op":"focus","id":"comment"}
```

Focusable nodes are:

- every `input`;
- `table` when `selectable:true`;
- `viewport` when `scrollable:true`.

Focus belongs to Runtime projection, not Document data. A focus mutation therefore does not by itself advance Document revision.

### Typical errors

- target missing;
- target exists but is not focusable.

The renderer may own additional local interaction state such as input caret or viewport offset. Those are not changed by the protocol `focus` contract except through the adapter's normal reconciliation rules.

---

## 6. `commit`

Use `commit` as a **publication barrier**.

```json
{"v":1,"seq":11,"op":"commit","frame":"choice-ready"}
```

`commit` is not a transaction around earlier mutations. Earlier accepted mutations are already authoritative.

The barrier asks for evidence that the semantic state through this mutation boundary entered the renderer publication path.

A resulting event has the shape:

```json
{
  "v": 1,
  "seq": 42,
  "ev": "committed",
  "revision": 7,
  "through_seq": 11,
  "frame": "choice-ready"
}
```

In the persistent-daemon implementation the path is:

```text
agent commit
    ↓
a2uid marks publication generation N pending
    ↓
snapshot N sent to a2ui
    ↓
Bubble Tea renders from that snapshot
    ↓
frame_published(N)
    ↓
Engine publishes generation N exactly once
    ↓
committed event
```

A socket write alone is not publication. A stale renderer acknowledgement cannot publish a newer generation.

`frame_published` is internal daemon/client IPC, not a seventh public A2UI mutation.

---

## User events

The renderer can turn local interaction into transport-independent events.

### Table activation

A selectable table with stable row IDs can emit:

```json
{"v":1,"seq":42,"ev":"select","id":"targets","row":1,"row_id":"production","action":"deployment.select"}
```

Moving the highlight with arrows does not itself emit a wire event. Explicit activation does.

### Input submission

Submitting a focused input emits a semantic `submit` event carrying the daemon-owned runtime input value.

### Host actions

An `actions` node can refer to an action ID, but an ID is not executable code. The host must have registered a corresponding handler. Unknown actions return `action.not_permitted`.

Registered handlers are still privileged host capabilities and belong to the security model, not to the six-mutation safety claim.

---

## Strict JSON and validation

A2UI's strict JSON entry points reject:

- duplicate object keys;
- unknown struct fields;
- malformed/trailing JSON values;
- unsupported protocol version;
- sequence mismatch between envelope and operation/event payload.

Mutation processing is candidate-based:

```text
current Document
  + mutation
  → candidate
  → schema/semantic/resource validation
  → commit candidate
```

If validation fails, the current authoritative Document must not be partially modified.

---

## Resource limits

The default runtime advertises finite limits, including:

```text
agent record bytes       1 MiB
retained Document bytes  8 MiB
nodes                     2048
max depth                 32
max children              512
text per node             256 KiB
total retained text       2 MiB
pending events            256
```

The local daemon/client snapshot protocol has a separate bounded record budget large enough to carry a valid retained Document plus JSON snapshot overhead. Do not confuse that local IPC budget with the public agent record limit.

---

## Renderer-neutral versus renderer-local state

A2UI keeps three different authorities:

```text
Document
  agent-owned semantic tree and props

Runtime projection
  focus, input values, semantic table selection

Adapter local
  caret, viewport offset/tail pin, animation phase, terminal size
```

Renderer presets (`minimal`, `dashboard`, `dense`) are local configuration. They do not change the public mutation vocabulary or Document semantics.

---

## What this guide does not define

This guide does not add:

- a shell action;
- template/macro expansion;
- mouse events;
- Waybar/D-Bus integration;
- clipboard integration;
- multi-client editing;
- disk persistence;
- a seventh mutation.

For exact validation rules and component props, use [`references/PROTOCOL.md`](../references/PROTOCOL.md) and [`assets/schema.json`](../assets/schema.json).
