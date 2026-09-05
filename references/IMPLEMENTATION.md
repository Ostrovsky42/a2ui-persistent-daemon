# A2UI v1 hardened Go implementation notes

## 1. Dependency rule

The semantic core is Go stdlib-only. Renderer frameworks are adapters.

Protocol correctness tests MUST run without Bubble Tea, Lip Gloss, CGO, network access, or a terminal.

## 2. Package responsibilities

### `protocol`

Owns wire/domain types, error codes, limits and component props validation. Unknown properties are rejected. `NormalizeProps` turns full upsert props into defaults + supplied fields, validates semantic presentation hints, and extracts one-shot input force policy.

### `document`

Owns the authoritative tree. `Apply` clones current state into a candidate, applies one operation, validates structure/resources (including aggregate retained Document bytes), and increments revision only after a real state change. `Node.ExplicitProps` retains top-level property presence so renderer variant defaults cannot overwrite an explicitly supplied value that happens to equal the normalized default. Full `upsert` resets this set; `props` patches add to it; one-shot `force` is excluded.

`box` and `viewport` are containers. Other node types are leaves.

Important invariant: callers must only replace their current document with the returned candidate when `err == nil`.

### `runtime`

Owns local state that an agent must not overwrite accidentally: input edit buffers, focus, deterministic action key index, projection/publication generation and event/action execution infrastructure.

`State.Reconcile` is itself candidate-based. A binding conflict does not partially update runtime state.

### `engine`

The reference integration boundary. It runs Document candidate apply, runtime reconciliation, and only then publishes the new Document. It also owns the reliable event broker and action dispatch integration.

### `session`

Owns A2UI hello/version/capability negotiation and reliable mutation sequence space. A forward sequence gap is fatal and closes the session.

### `wire`

Strict JSON and bounded NDJSON. It rejects duplicate JSON keys and unknown top-level fields, distinguishes clean EOF from reader failure, and can normalize legacy operation records into hardened envelopes.

### `layout`

Renderer-neutral layout math. It owns box-model budgeting and deterministic row allocation through `FlexConstraint` / `AllocateRow`. Allocation honors basis, minimum/maximum widths, grow weights, gap accounting and deterministic remainder distribution. It reports when declared minimum widths cannot fit so the renderer can apply an explicit responsive policy without mutating the Document.

### `external`

Thread-affine actor. Its goroutine calls `runtime.LockOSThread` before executing injected native work. A timeout after request submission marks actor health `DEGRADED`; it does not pretend the native call was cancelled.

### `ipc`

Versioned, bounded, strict local daemon/client control protocol over Unix streams. It is independent of public A2UI protocol versioning and carries atomic presentation snapshots plus semantic interaction commands.

### `daemon`

Long-lived owner of `Session`, `Engine`, event broker and action registry. It accepts one interactive renderer lease, forwards public Agent envelopes into the existing semantic core, and never calls `Publish()` merely because snapshot bytes were written.

### `transport/httpstream`

Streaming `net/http.Handler`. No third-party HTTP/2 package is needed for the handler itself. Production TLS server configuration determines HTTP/2 negotiation.

### `transport/mcp`

Stateless JSON-RPC mapping helpers for current MCP integration. A2UI session is an explicit application handle.

### `transport/udp`

Codec for telemetry-only datagrams. It intentionally cannot encode document/control envelope kinds.

## 3. Bubble Tea Renderer V2

`adapter/bubbletea` is a projection/rendering adapter. It does not own protocol existence or semantic state.

The adapter reads immutable engine snapshots and renders them through these layers:

```text
Document + Runtime projection
        -> semantic presentation resolution
        -> layout allocation / responsive transform
        -> renderer-local preset + animation state
        -> Lip Gloss frame
```

Renderer-side presets are `minimal`, `dashboard`, and `dense`. Presets are application configuration, not protocol props. The same Document can therefore render differently without changing revision, bindings or semantic content.

The implementation is split by responsibility:

- `preset.go` — preset identities and renderer-local presentation profiles;
- `render_state.go` — deterministic ephemeral phase/cursor inputs;
- `renderer.go` — tree traversal, box composition, flex and responsive composition;
- `render_components.go` — text/input/actions/progress/table/viewport component rendering;
- `render_helpers.go` — terminal-width-aware text fitting/wrapping and prop helpers;
- `styles.go` — renderer-local theme roles;
- `model.go` — Bubble Tea lifecycle, one animation scheduler and publication integration;
- `key_router.go` — deterministic key routing and focused component controllers;
- `interaction_state.go` — bounded adapter-local caret/viewport mechanics;
- `render_result.go` — frame plus renderer-derived viewport visual-line metrics.

### Authority rules

Bubble Tea local state may contain terminal size, input caret, viewport offset/tail pin, animation phase and cursor blink. Semantic table selection, input value and focus belong to renderer-independent Runtime projection. The adapter MUST NOT store an authoritative copy of Document or semantic interaction state.

Responsive `row -> column` stacking is a visual transform only. It never reparents nodes or increments Document revision.

Loading/spinner/cursor motion is driven by one event-driven scheduler for the whole model. Static documents schedule no animation tick. There is no timer per component and no permanent 60 Hz loop.

### Publication barrier

`View()` calls `Engine.Publish()` only when `Engine.NeedsPublish()` reports dirty/pending authoritative projection work. Animation-only redraws modify only renderer-local phase and MUST NOT publish, increment Document revision, or synthesize `committed` events.

The intended flow remains:

```text
commit accepted
  -> pending publication
  -> renderer produces visible frame
  -> Engine.Publish()
  -> committed event
```

## 4. Presentation semantics

A2UI v1 Renderer V2 adds bounded semantic presentation hints instead of a CSS language:

- text variants: `body|title|subtitle|label|code|muted`;
- box variants: `plain|panel|card|section`;
- actions variants: `inline|toolbar|list`;
- progress variants: `bar|compact|spinner`;
- progress states: `normal|loading|success|error`;
- table variants: `normal|compact|dense`;
- common flex hints: `grow`, `basis`, `min_width`, `max_width`;
- box `responsive: none|stack` and `align: start|center|end|stretch`;
- viewport `follow_tail`.

The protocol remains strict: unknown fields and invalid enum values fail before authoritative mutation. Presentation precedence is explicit protocol property, then variant/preset default, then global preset default. Because explicitness can change a rendered result, changing only explicit-property presence is a real Document semantic change and advances revision.

## 5. Backpressure

Critical event enqueue is fallible. Callers MUST inspect `*protocol.Error`. `runtime.backpressure_exceeded` is preferable to silent action/submit loss.

Coalescible environment/state events and lossy telemetry use separate storage policy.

## 6. Native/C driver integration

`external.Actor` serializes calls on one OS thread. Inject actual CGO/purego calls through `external.Executor`.

A Go context timeout only bounds caller waiting. It cannot forcibly preempt arbitrary native code. After an in-flight timeout the actor is degraded, and the application decides whether to restart the native subsystem/process.

For truly untrusted/hang-prone native code, process isolation is stronger than an in-process goroutine.

## 7. Test strategy

Required release gates:

```bash
go test ./...
go test -race ./...
go vet ./...
```

Renderer-focused tests additionally prove:

- one canonical Document produces distinct frames for all three presets while retaining the same semantic content;
- wide responsive rows remain horizontal and constrained narrow rows stack vertically;
- viewport clipping/follow-tail is deterministic;
- animation phase changes frames without changing Document revision;
- animation ticks never fabricate publication;
- static Documents need no animation scheduler.

`document/model_test.go` runs a deterministic long transition sequence and checks structural/resource invariants after every accepted or rejected operation.

Fuzz targets cover raw wire decoding and reducer application. Longer fuzzing can be run with:

```bash
go test ./wire -fuzz=FuzzDecodeRecord -fuzztime=30s
go test ./document -fuzz=FuzzApplyNeverBreaksDocumentInvariants -fuzztime=30s
```

`conformance/testdata/basic.ndjson` is a record/replay fixture independent of any renderer.

## 8. Evolution rule

Do not add more widget types until protocol/reducer/session conformance remains green. New semantic surface should first appear as protocol tests/schema changes, then Document/runtime behavior if needed, then renderer implementation.

Do not turn presentation growth into a CSS engine without a separate architecture checkpoint. Web rendering, manual viewport scrolling, mouse input, absolute/percentage positioning and an animation DSL remain out of Renderer V2 scope.

## 9. Interactive Runtime V3

V3 introduces a strict ownership split:

| State | Owner | Wire-visible | Publication dirty |
|---|---|---:|---:|
| table rows / stable row IDs | Document | yes | yes |
| viewport `follow_tail` / `scrollable` policy | Document | yes | yes |
| focused node | Runtime projection | only when agent uses `focus` op | agent focus: yes; local focus: no |
| input value | Runtime projection | submit boundary | no for local edits |
| table selected row | Runtime projection | only on explicit `select` activation | no |
| input caret | Bubble Tea adapter | no | no |
| viewport offset / tail pin | Bubble Tea adapter | no | no |
| animation phase | Bubble Tea adapter | no | no |
| terminal dimensions | Bubble Tea adapter | no | no |

`runtime.TableSelection` carries numeric index plus optional stable `RowID`. Reconciliation follows stable IDs across Document row reorder and removes/clamps selection when capabilities/data change. Bubble Tea renders the immutable selection snapshot and never owns it.

`engine.PresentationSnapshot()` clones Document, focus, input values and table selections under one Engine lock. Renderers should consume this snapshot rather than issuing several independent state reads. Returned maps/documents are copies and cannot mutate Engine internals.

Manual viewport scrolling is deliberately adapter-local. `RenderFrame` accepts immutable `InteractionState` and returns `ViewportMetrics` (`TotalLines`, `VisibleLines`, `MaxOffset`, resolved `Offset`). The controller uses those metrics, so wrapping/resize calculations remain renderer-owned rather than duplicated in the Model.

Key routing is process control, focus traversal, focused component, then global action binding. This prevents input text from accidentally triggering an action hotkey. Table activation emits a critical `select` event; row movement does not.

## 10. Persistent daemon/client process model

Epic 1 moves semantic authority out of the terminal process. Ownership is explicit:

| State / subsystem | Owner | Survives client detach |
|---|---|---:|
| Agent connection / Session | daemon | yes |
| Engine / Document | daemon | yes |
| Runtime focus / input value / table selection | daemon | yes |
| Event broker / ActionRegistry | daemon | yes |
| Publication generation / pending commits | daemon | yes |
| Bubble Tea renderer | client | no |
| input caret | client | no |
| viewport offset / tail pin | client | no |
| animation phase / terminal size | client | no |

The Bubble Tea adapter depends on a narrow `SemanticController`. `localSemanticController` wraps an in-process Engine for `a2ui-runner`; `ipc.Client` implements the same boundary for `cmd/a2ui`. A remote model never creates a fallback Engine after disconnection because that would fork authority.

### Local IPC

`ipc` uses strict bounded NDJSON over a Unix domain socket. Default message budget is 16 MiB so one maximum retained Document snapshot can fit with JSON/runtime metadata overhead. Writer calls are serialized and daemon update notification is a one-element coalescing channel; slow clients therefore cannot build unbounded snapshot history. Socket writes occur after `PresentationSnapshot()` has released the Engine mutex.

The default path is `$XDG_RUNTIME_DIR/a2ui/a2ui.sock`, falling back to `/tmp/a2ui-$UID/a2ui.sock`. Parent directory permissions are forced to `0700`, socket/startup lock to `0600`. A per-path advisory startup lock serializes stale-socket probe/remove/bind, preventing concurrent daemons from unlinking the winner. `UnixListener` owns unlink-on-close; shutdown never performs a later pathname-based remove.

### Single interactive lease

Exactly one interactive client can hold semantic controller authority. All connections first send `hello`; lease acquisition happens only after a valid handshake. A second valid client receives `ipc.client_busy`. EOF, broken connection, explicit detach or handler cancellation releases the lease.

### Snapshot-on-change

Initial attach and semantic updates send an atomic `Engine.PresentationSnapshot()`. The snapshot is a projection cache in the client, not authority. Update signals coalesce to the latest state; no generic second delta protocol is introduced. Renderer-local caret/viewport state is intentionally rebuilt on reattach.

### Remote publication barrier

Snapshot delivery is not publication. `PresentationSnapshot` carries `PublicationGeneration` and `PublicationPending`. A client that renders a pending generation sends `frame_published(generation)`. The daemon conditionally invokes `Engine.PublishGeneration()` only for the exact current token. Duplicate ACK is a no-op; stale ACK cannot publish a newer frame. If a client disconnects before ACK, publication remains pending and the next client receives/ACKs that generation. With no client attached, the daemon never fakes visual publication.

### Error separation

Fatal transport/codec failures populate `ConnectionError()` and produce the Bubble Tea disconnected state. Recoverable daemon diagnostics such as `ipc.stale_publication` remain inspectable through `Err()` but do not mark the semantic controller disconnected.
