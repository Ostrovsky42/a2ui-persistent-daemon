# A2UI v1 Hardening Design

## Goal

Turn the original NDJSON/Bubble Tea sketch into a small transport-neutral state-synchronization protocol whose core semantics are deterministic, bounded, testable without a renderer, and suitable for stdio/NDJSON, HTTP streaming/HTTP2, an MCP bridge, and a deliberately restricted UDP telemetry profile.

## Architectural laws

1. Transport does not define A2UI semantic behavior.
2. `document.Document` is the only authoritative UI state.
3. A rejected mutation changes no authoritative state.
4. Widget instances, focus, input edit buffers, selection and publication bookkeeping are runtime-local projection state.
5. All structural and action-resolution transitions are deterministic.
6. Critical events are never silently dropped.
7. Retained and in-flight resources are finite and negotiated.
8. Version, capabilities and limits are validated before a hardened reliable session becomes READY.
9. Every protocol error has a stable code and a defined recovery level.
10. Conformance is demonstrated by tests, model sequences, fuzzing and record/replay rather than inferred from documentation.

## Layering

```text
transport
   ↓
wire / framing / strict JSON
   ↓
session / negotiation / reliable sequence
   ↓
protocol operation validation
   ↓
transactional Document reducer
   ↓
runtime-local projection
   ↓
renderer adapter
```

UDP is a side profile, not a substitute reliable transport:

```text
UDP datagram → telemetry envelope only → lossy consumer
```

Document mutations, submit/action traffic and commit barriers are not permitted in the UDP profile.

## Authoritative state and transactions

A mutation is applied to a candidate clone. Props are normalized and validated, structural/resource invariants are checked, then and only then is the candidate returned as the next Document. A failure leaves the old Document unchanged.

`upsert` is a full props replacement. `props` is a shallow top-level merge. Only `box` can own children. Root is immutable. Type changes recreate runtime-local state for that ID. An invalid create cannot poison/reserve the ID.

Document revision advances only when authoritative Document state actually changes. Focus, one-shot input overwrite policy and other local changes instead advance a runtime projection generation so a renderer knows a new frame is required without lying about Document revision.

## Publication barrier

`commit` is not a transaction. Earlier accepted mutations are already authoritative. It queues a publication barrier. A renderer renders a current snapshot and calls `Engine.Publish()` only after the frame is made visible. `Publish()` then emits reliable `committed` events containing the represented Document revision and `through_seq`.

The runtime does not require a permanent 60 Hz tick. Dirty projection generation and pending commit barriers determine whether publication work exists.

## Events and backpressure

Critical events (`submit`, action result, protocol/application error, `committed`) use a bounded reliable broker. Capacity exhaustion is an explicit `runtime.backpressure_exceeded` failure. State/environment events may be coalesced, and telemetry may be lossy where explicitly documented.

Pending commit barriers are admission-controlled against event capacity so the engine cannot accept a commit batch it can never publish.

## Actions and native execution

Remote input references symbolic local action IDs only. Registration rejects empty IDs and nil handlers. Bindings are indexed deterministically in tree preorder and conflicting keys reject the candidate.

Action timeout covers both waiting for an inflight slot and handler execution. A handler panic is recovered into `action.failed` rather than terminating the process.

Thread-affine/native calls run through `external.Actor`, which locks one goroutine to one OS thread. A panic is contained and degrades the actor. A timeout after submission also degrades the actor because Go context cancellation cannot prove a blocking native call was interrupted.

## Wire and sessions

The hardened envelope contains protocol version, explicit A2UI session handle, message kind, sequence where required, and a kind-specific payload. Strict JSON decoding rejects unknown fields, duplicate object keys at any nesting depth, malformed/trailing values, missing required fields and discriminator/payload mismatches.

Reliable mutation sequence starts at 1 and is contiguous. Old sequence values are duplicates and do not apply twice. Forward gaps are fatal for that reliable session. Outbound event sequence and Document revision are separate counters.

`hello`/`hello_ack` negotiate protocol version, component capabilities, features and finite effective limits.

## Transport profiles

### NDJSON / stdio

Canonical debugging, recording and replay format. One bounded JSON record per line. A malformed line is recoverable because framing remains known; transport read failure is distinct from clean EOF.

### HTTP streaming / HTTP2

A streaming `net/http.Handler` consumes and emits NDJSON envelopes. Responses are validated before the first successful HTTP status is committed. Invalid local outbound data before stream start produces 5xx; after stream start it becomes an explicit A2UI transport error where possible. Each record is flushed. The handler is HTTP-version-neutral and works over Go's HTTP/2 TLS server path.

### MCP bridge

A2UI envelopes map to strict JSON-RPC messages. The bridge targets MCP protocol version `2026-07-28` routing headers (`MCP-Protocol-Version`, `Mcp-Method`). A2UI session state remains an explicit application handle and is not hidden in an MCP transport session.

### UDP

Telemetry-only, one A2UI envelope per datagram, default 1200-byte budget, no fragmentation/reassembly. Sequence numbers allow detection of loss/reordering but do not imply retransmission.

## Resource model

Effective limits bound at least message bytes, retained Document bytes, nodes, depth, children, retained text per node, retained text total, table rows/columns, pending reliable events/mutations, inflight actions and UDP datagram size. The aggregate Document budget includes IDs, parent/child references, props and retained text so many individually legal nodes cannot produce unbounded retained memory.

## Verification strategy

The core must build and test using Go stdlib only. Required release gates are:

```bash
gofmt check
go test ./...
go vet ./...
go test -race ./...
go test ./wire -run '^$' -fuzz '^FuzzDecodeRecord$' -fuzztime=3s
go test ./document -run '^$' -fuzz '^FuzzApplyNeverBreaksDocumentInvariants$' -fuzztime=3s
```

In addition, JSON examples must validate against `assets/schema.json`, and the final distributable archive must pass `unzip -t`.
