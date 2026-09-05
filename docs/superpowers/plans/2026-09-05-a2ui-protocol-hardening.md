# A2UI Protocol Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce a buildable A2UI v1 core with transactional reconciliation, bounded sessions/events, deterministic actions/focus, hardened NDJSON, and stdlib transport profiles for HTTP/MCP/UDP integration.

**Architecture:** The Go core is stdlib-only and renderer-neutral. `document.Document` is authoritative; protocol operations become validated candidate transitions before commit, and runtime/local state is kept separately. Transport adapters exchange typed envelopes and cannot weaken semantic guarantees.

**Tech Stack:** Go 1.23 standard library; JSON/NDJSON; net/http; net/UDP; Go fuzz/race/vet. Bubble Tea remains an optional/documented renderer integration target rather than a core dependency.

**Spec:** `docs/superpowers/specs/2026-09-05-a2ui-protocol-hardening-design.md`

## Global Constraints

- Keep the original six A2UI v1 document operations and seven node types.
- Critical events must never be silently dropped.
- Failed mutations must not alter the authoritative document.
- UDP v1 is telemetry-only and never carries authoritative mutations/actions/commit.
- All resource use exposed by untrusted protocol input is bounded by configured limits.
- Default build and test of the core must require no third-party modules.

---

### Task 1: Module and protocol model

**Files:**
- Create: `go.mod`
- Create: `protocol/types.go`
- Create: `protocol/errors.go`
- Create: `protocol/props.go`
- Test: `protocol/protocol_test.go`

**Interfaces:**
- Produces `protocol.Operation`, `protocol.Envelope`, `protocol.Event`, `protocol.Limits`, `protocol.ValidateOperation` and typed props validation used by every later task.

- [ ] Write tests proving node/type/prop validation, unknown property rejection, valid color tokens, and `force` extraction.
- [ ] Run focused tests and verify RED because protocol package does not exist.
- [ ] Implement the minimum protocol model and validators.
- [ ] Run focused tests and verify GREEN.

### Task 2: Transactional document reducer

**Files:**
- Create: `document/document.go`
- Create: `document/reducer.go`
- Create: `document/invariants.go`
- Test: `document/reducer_test.go`

**Interfaces:**
- Consumes `protocol.Operation`, `protocol.Limits`.
- Produces `document.Document`, `document.Apply(Document, Operation, Limits) (Document, Effect, *protocol.Error)`.

- [ ] Write RED tests for `props(box)`, failed-mutation rollback, parent-must-be-box, full replace, shallow merge, invalid-to-valid recovery, type replacement, remove/root invariants, depth/node limits, and text retention.
- [ ] Run focused tests and verify expected failures.
- [ ] Implement copy-on-write candidate transitions and invariant checks.
- [ ] Run tests and verify GREEN.

### Task 3: Runtime local state, focus, bindings, commit publication

**Files:**
- Create: `runtime/state.go`
- Create: `runtime/focus.go`
- Create: `runtime/bindings.go`
- Create: `runtime/publication.go`
- Test: `runtime/runtime_test.go`

**Interfaces:**
- Consumes committed documents/effects.
- Produces deterministic focus reconciliation, deterministic action binding lookup, one-shot input overwrite policy, dirty/publication revisions, and commit acknowledgements.

- [ ] Write RED tests for stale focus after remove/non-focusable/type-change, tree-order focus, duplicate action key rejection, one-shot force behavior, and observable commit barrier.
- [ ] Run focused tests and verify RED.
- [ ] Implement runtime state reconciliation and publication scheduling state.
- [ ] Run tests and verify GREEN.

### Task 4: Reliable event broker and action registry

**Files:**
- Create: `runtime/events.go`
- Create: `runtime/actions.go`
- Test: `runtime/events_actions_test.go`

**Interfaces:**
- Produces bounded reliable/coalescing event queues and `ActionRegistry` with safe registration/dispatch and invocation IDs.

- [ ] Write RED tests that critical events cannot silently drop, resize coalesces, overflow is observable, nil handlers are rejected, unknown actions fail, and handler timeout emits deterministic error.
- [ ] Run focused tests and verify RED.
- [ ] Implement broker and registry.
- [ ] Run tests and verify GREEN.

### Task 5: Session and wire/NDJSON

**Files:**
- Create: `session/session.go`
- Create: `wire/json.go`
- Create: `wire/ndjson.go`
- Test: `session/session_test.go`
- Test: `wire/ndjson_test.go`

**Interfaces:**
- Produces negotiation, reliable mutation sequence checking, document revisions, legacy operation normalization, scanner error distinction, and record-size limits.

- [ ] Write RED tests for hello negotiation, duplicate/gap handling, clean EOF vs scanner error, malformed-line recovery, legacy v1 normalization, and oversized-record rejection.
- [ ] Run focused tests and verify RED.
- [ ] Implement session and codec/stream readers.
- [ ] Run tests and verify GREEN.

### Task 6: Transport profiles

**Files:**
- Create: `transport/httpstream/httpstream.go`
- Create: `transport/mcp/mcp.go`
- Create: `transport/udp/udp.go`
- Test: `transport/httpstream/httpstream_test.go`
- Test: `transport/mcp/mcp_test.go`
- Test: `transport/udp/udp_test.go`

**Interfaces:**
- HTTP exposes ordered JSON/NDJSON session ingress/egress over `net/http` (HTTP/2-capable when hosted with Go TLS server).
- MCP maps A2UI session messages to JSON-RPC notifications/methods without changing A2UI semantics.
- UDP encodes one telemetry envelope per datagram and rejects authoritative/control kinds.

- [ ] Write RED tests for reliable HTTP ordering, MCP request/notification mapping, UDP allowlist, datagram size, sequence presence, and no fragmentation.
- [ ] Run focused tests and verify RED.
- [ ] Implement stdlib transports.
- [ ] Run tests and verify GREEN.

### Task 7: CGO/external actor lifecycle

**Files:**
- Create: `external/actor.go`
- Test: `external/actor_test.go`

**Interfaces:**
- Produces a thread-affine actor lifecycle with READY/DEGRADED/CLOSED states, bounded request queue, timeout semantics, and close/drain behavior. The actor accepts a Go callback so tests require no C compiler.

- [ ] Write RED tests for serialized execution, timeout -> DEGRADED, bounded queue, and Close behavior.
- [ ] Run focused tests and verify RED.
- [ ] Implement actor with `runtime.LockOSThread`.
- [ ] Run tests and verify GREEN.

### Task 8: Conformance, fuzz, documentation, archive gate

**Files:**
- Create: `conformance/conformance_test.go`
- Create: `conformance/testdata/basic.ndjson`
- Create: `conformance/testdata/basic.want.json`
- Create: `.github/workflows/ci.yml`
- Modify: `README.md`
- Modify: `SKILL.md`
- Modify: `references/PROTOCOL.md`
- Modify: `references/IMPLEMENTATION.md`
- Replace: `assets/schema.json`
- Update: `assets/example-session.ndjson`

**Interfaces:**
- Produces one normative documentation set, executable schema parity, conformance replay, and CI verification commands.

- [ ] Add conformance replay and fuzz targets, then run them.
- [ ] Make schema/documentation match implementation semantics exactly.
- [ ] Run `gofmt -w` on all Go files.
- [ ] Run `go test ./...` and require exit 0.
- [ ] Run `go test -race ./...` and require exit 0.
- [ ] Run `go vet ./...` and require exit 0.
- [ ] Pack the verified tree as `a2ui-v1-hardened.zip`.
