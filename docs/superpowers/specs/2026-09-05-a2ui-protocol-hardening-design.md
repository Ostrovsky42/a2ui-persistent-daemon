# A2UI Protocol Hardening Design

## Goal

Turn the A2UI v1 sketch into a deterministic, bounded, transport-neutral protocol/runtime core that can be tested without a terminal renderer and carried over stdio/NDJSON, HTTP streaming, MCP/JSON-RPC integration, and a deliberately restricted UDP telemetry profile.

## Non-goals

A2UI v1 does not add arbitrary remote code execution, WASM widgets, modal/window systems, distributed collaborative editing, custom scripts, or reliable authoritative mutation delivery over UDP.

## Architecture

A2UI is split into six layers:

1. **transport** — bytes, datagrams, HTTP requests, or MCP messages;
2. **wire** — envelopes and JSON encoding/framing;
3. **session** — negotiation, sequence spaces, limits, capabilities, lifecycle;
4. **protocol** — typed operations/events and validation;
5. **document/reconciler** — authoritative immutable-by-commit UI state transitions;
6. **runtime projection** — focus, widget-local state, event broker, actions, rendering adapters.

The authoritative state is `document.Document`. Widget instances are projections and never determine node existence.

## Mutation invariant

Every operation is applied transactionally:

`current document + operation -> candidate -> validation -> invariant check -> commit revision`.

If validation fails, the current document and revision MUST remain byte-for-byte semantically unchanged.

## Document invariants

- `root` always exists and cannot be removed.
- Node IDs are unique.
- Every non-root node has exactly one existing parent.
- Only `box` nodes may have children.
- The tree is acyclic.
- Negotiated depth/node/resource limits are enforced before commit.
- A type change is legal but recreates runtime-local state for that node.
- `upsert` replaces props completely; `props` performs a shallow top-level merge.
- Unknown properties are rejected in v1.
- Invalid styles/colors reject the mutation; previously accepted state is preserved.

## Input local state

Persistent props do not contain one-shot control flags. `force` remains accepted on the wire for v1 compatibility, but is extracted as mutation policy and is not retained in document props. `value` is only authoritative on creation unless `force:true` is present.

## Session

Session lifecycle: `NEW -> READY -> DRAINING -> CLOSED`.

The first semantic exchange is `hello` / `hello_ack`. Negotiation selects protocol version and publishes capabilities and hard limits. Reliable profiles use separate monotonically increasing mutation and event sequence spaces. Duplicate mutation sequence numbers are idempotently ignored; gaps on reliable profiles are session errors.

Document revision is independent from message sequence numbers and increments only after accepted state-changing mutations.

## Commit

`commit` is a publication barrier, not a document transaction. Mutations before it are already authoritative. A commit requests publication through the current document revision and yields a `committed` event containing `through_seq`, `revision`, and optional frame label.

## Events and backpressure

Events are classified:

- **critical/reliable**: `submit`, `action`, `action_result`, `error`, `committed`;
- **coalescible state/environment**: `resize`, `focus`, `select`;
- **lossy telemetry**: explicitly marked telemetry only.

Critical events MUST NOT be silently dropped. A bounded reliable queue that cannot accept another critical event returns `backpressure_exceeded` and makes the failure observable.

## Actions

Action registration validates non-empty IDs and non-nil handlers. Bindings are indexed deterministically in document tree order. Duplicate keys at the same scope are rejected with `binding_conflict` instead of relying on Go map iteration. Action execution has an invocation ID and explicit timeout. Context cancellation does not claim to interrupt a blocking C call; CGO actors expose `DEGRADED` health after timeout.

## Layout contract

Layout is renderer-independent. The core computes a box model from terminal width/height, border, padding, direction, and gap. Border and padding consume available space. Row and column gaps are both represented. Viewport height is clamped to available terminal height. Renderer adapters consume the resulting layout projection.

## Wire and transports

The canonical debug codec is newline-delimited JSON. Transport is not protocol semantics.

### Reliable profiles

stdio/NDJSON, HTTP streaming, and MCP integration MUST preserve ordered reliable envelope delivery or surface a transport/session failure.

### UDP profile

UDP is **telemetry-only** in v1. It MUST NOT carry authoritative document mutations, submit events, action commands/results, or commit barriers. Each datagram is one complete envelope and includes protocol version, session ID, channel, and datagram sequence. A payload that exceeds configured datagram size is rejected; no A2UI fragmentation/reassembly exists.

## Resource limits

Negotiated defaults are finite: message bytes, nodes, depth, children per node, text bytes per node, total retained text, table rows, table columns, pending reliable events, pending mutations, and inflight actions. Limits are checked before allocation/commit where practical.

## Error taxonomy

Errors use stable namespaced codes and indicate recoverability. Validation/application errors are recoverable; version negotiation failure, reliable sequence corruption, framing corruption without recoverable record boundaries, and internal invariant failure are session-fatal.

## Compatibility

A2UI v1 keeps the six original document operations (`upsert`, `props`, `text`, `remove`, `focus`, `commit`) and seven original node types. The outer session/wire envelope is hardened, but the NDJSON decoder accepts legacy v1 operation records and normalizes them into the new internal envelope for migration.

## Verification

The release requires:

- unit tests for reducer semantics and each previously identified bug;
- model/property tests asserting invariants after arbitrary operation sequences;
- fuzz targets for decoder and reconciler;
- race-safe queues and lifecycle tests;
- conformance fixtures with input, expected final document, and expected events;
- `go test ./...`, `go test -race ./...`, `go vet ./...` on the stdlib-only core.
