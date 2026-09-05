---
name: a2ui-v1
summary: Generate deterministic A2UI v1 UI operations without executable code.
---

# A2UI v1 agent rules

Use A2UI to describe UI state. Do not generate Go source, shell commands, terminal escape sequences, arbitrary HTML, or OS commands.

Normative source: `references/PROTOCOL.md`.

## Allowed operations

Exactly:

```text
upsert
props
text
remove
focus
commit
```

Allowed node types:

```text
box
text
viewport
table
input
actions
progress
```

`box` and `viewport` may contain children. `text`, `table`, `input`, `actions`, and `progress` are leaves.

## Mutation semantics

`upsert` is full props replacement. Missing fields return to defaults.

`props` is shallow top-level merge. Do not assume nested merge.

Failed mutations do not partially apply. Read returned error and correct the next operation.

Unknown fields are errors in v1. Spell props exactly as defined.


## Semantic presentation

Choose semantic intent, not a renderer theme. Never send a `preset` property. Presets such as `minimal`, `dashboard`, and `dense` are host/renderer configuration.

Allowed semantic variants:

```text
text:     body | title | subtitle | label | code | muted
box:      plain | panel | card | section
actions:  inline | toolbar | list
progress: bar | compact | spinner
table:    normal | compact | dense
```

Progress `state` is `normal | loading | success | error`. Use `state:"loading"` to express semantic activity; do not send animation frames or timing.

For row children, optional `flex` accepts only `grow`, `basis`, `min_width`, and `max_width`. A row box may use `responsive:"stack"` so the renderer can switch the visual layout to a column when declared minimum widths do not fit. This does not mutate the Document tree.

Viewport may use `follow_tail:true` for streaming/log presentation and `scrollable:true` when local manual navigation should be allowed. `scrollable` changes focusability/capability, not the authoritative viewport offset.

## Input ownership

Do not repeatedly send `value` while a user is typing.

To explicitly overwrite/clear an existing input once, send `force:true` in that same input mutation. `force` is one-shot; do not assume it remains enabled.

## Focus

Focusable nodes are:

- `input`;
- `table` with `selectable:true`;
- `viewport` with `scrollable:true`.

If a focused node disappears or stops being focusable, runtime chooses the next focusable node deterministically. Do not encode caret position or viewport scroll offset in the Document.

## Interactive tables

For agent-updated selectable tables, prefer stable `row_ids`. When supplied, `row_ids` MUST be unique, non-empty, and have the same length as `rows`. Use optional table `action` to describe activation intent. Runtime preserves the selected semantic row by `row_id` across reordering when possible.

User row movement is local and does not emit an event for every arrow key. Explicit activation (`Enter` in a terminal host) emits a critical `select` event carrying `row`, optional `row_id`, and `action`.

## Actions

Use only action IDs advertised/registered by the host. Never invent shell/file/network commands.

Keep action keys unique within the v1 screen scope. Duplicate keys reject the candidate.

## Commit

`commit` is a publication barrier. Use it after a meaningful construction/update batch when you need acknowledgement that the current Document revision was published.

Do not treat commit as rollback/transaction control.

## Streaming text

Use `text` only for `text` and `viewport` nodes. Text retention is bounded. Do not stream unbounded logs indefinitely.

## Transport

Normal authoritative UI mutations require a reliable A2UI profile (stdio/NDJSON, HTTP streaming, or host/MCP bridge).

UDP is telemetry-only. Never send document mutations, submit/action control, or commit through UDP.


## Host-local daemon mode

Persistent daemon/client IPC is host implementation detail. An agent MUST continue emitting only the public A2UI envelopes/operations defined by `references/PROTOCOL.md`; it MUST NOT generate `ipc` messages such as `frame_published`, `detach`, or daemon `interaction` records.

The daemon may keep the same A2UI Session/Document/Runtime alive while terminal clients detach and reattach. Renderer publication acknowledgements are produced by the trusted host client, not by the agent.

## Hardened session

When a host asks for hardened envelopes, wait for/perform A2UI hello negotiation and honor advertised limits/capabilities. Reliable mutation sequence numbers are contiguous; never intentionally skip one.

## Minimal example

```json
{"v":1,"seq":1,"op":"upsert","id":"main","type":"box","parent":"root","props":{"dir":"col","gap":1}}
{"v":1,"seq":2,"op":"upsert","id":"title","type":"text","parent":"main","props":{"text":"Status","style":{"fg":"primary","bold":true}}}
{"v":1,"seq":3,"op":"upsert","id":"cmd","type":"input","parent":"main","props":{"placeholder":"command","action":"cmd.submit"}}
{"v":1,"seq":4,"op":"focus","id":"cmd"}
{"v":1,"seq":5,"op":"commit","frame":"boot"}
```
