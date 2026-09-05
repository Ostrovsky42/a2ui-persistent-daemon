# A2UI v1 Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the original renderer-coupled sketch with a verified transport-neutral A2UI v1 semantic core and hardened transport profiles.

**Architecture:** Keep `Document` authoritative and transactional, keep runtime-local UI state in a separate projection, validate strict envelopes/session sequencing before reducer application, and expose independent reliable and lossy transport profiles. Renderer frameworks remain adapters outside protocol correctness.

**Tech Stack:** Go 1.23+, standard library core, JSON/NDJSON, `net/http`, JSON-RPC bridge helpers, UDP datagrams, Go tests/race/vet/fuzz, JSON Schema Draft 2020-12.

**Spec:** `docs/superpowers/specs/2026-09-05-a2ui-v1-hardening-design.md`

## Global Constraints

- Semantic core MUST use only the Go standard library.
- A rejected mutation MUST NOT alter authoritative Document state.
- Critical events MUST NOT be silently dropped.
- UDP v1 MUST remain telemetry-only and MUST NOT implement fragmentation/reassembly.
- Renderer-local state MUST NOT become protocol authority.
- Resource limits MUST be finite and advertised by hardened session negotiation.

---

### Task 1: Protocol types, props and limits

**Files:** `protocol/types.go`, `protocol/props.go`, `protocol/validate.go`, `protocol/errors.go`, `protocol/*_test.go`

**Produces:** validated node/operation/envelope types, component props normalization, one-shot input force extraction, effective finite limits and stable protocol errors.

- [x] Write failing tests for unknown props, invalid component values, explicit `props:null`, negative operation sequence, hello/hello-ack uniqueness and finite limits.
- [x] Run targeted tests and observe the expected RED behavior.
- [x] Implement strict validation and effective defaults.
- [x] Run protocol tests to GREEN.

### Task 2: Transactional Document reducer

**Files:** `document/document.go`, `document/reducer.go`, `document/invariants.go`, `document/*_test.go`

**Produces:** `document.Apply(current, op, limits)` returning a validated candidate and effects without partial mutation.

- [x] Write RED tests for `props(box)`, leaf parents, invalid-create recovery, full-replace props, rollback on invalid merge, root immutability, no-op revision behavior and aggregate retained-memory limits.
- [x] Implement clone/apply/validate/commit reducer semantics.
- [x] Add structural invariant checks and deterministic long model-sequence verification.
- [x] Run document tests to GREEN.

### Task 3: Runtime projection, focus and bindings

**Files:** `runtime/state.go`, `runtime/bindings.go`, `runtime/publication.go`, `runtime/runtime_test.go`

**Produces:** runtime-local input/focus/binding state and independent render-generation publication tracking.

- [x] Add RED tests for stale focus, one-shot force, deterministic binding conflicts, focus-only publication and local-input publication.
- [x] Implement candidate runtime reconciliation and projection generation.
- [x] Run runtime tests to GREEN.

### Task 4: Reliable events and actions

**Files:** `runtime/events.go`, `runtime/actions.go`, `runtime/events_actions_test.go`

**Produces:** bounded reliable/coalescible event broker and guarded local ActionRegistry.

- [x] Add RED tests for critical event saturation, timeout while waiting for inflight capacity, handler timeout and handler panic.
- [x] Implement explicit backpressure, invocation IDs, whole-dispatch timeout and panic containment.
- [x] Run runtime tests to GREEN.

### Task 5: Engine and real commit publication barrier

**Files:** `engine/engine.go`, `engine/engine_test.go`

**Produces:** integrated Document/runtime/event engine with renderer-controlled `Publish()` and bounded pending commit admission.

- [x] Add RED tests proving commit does not acknowledge before actual publication.
- [x] Add RED test proving multiple commits cannot form a permanently unpublishable batch.
- [x] Implement `NeedsPublish()`/`Publish()` and commit admission control.
- [x] Run engine tests to GREEN.

### Task 6: Session negotiation and sequence spaces

**Files:** `session/session.go`, `session/session_test.go`

**Produces:** hello negotiation, explicit A2UI session state and reliable mutation sequence validation.

- [x] Add RED tests for unsupported/invalid hello, duplicate mutations, zero sequence and forward gaps.
- [x] Implement version/capability/limits negotiation and reliable sequencing.
- [x] Run session tests to GREEN.

### Task 7: Strict wire and NDJSON

**Files:** `wire/json.go`, `wire/ndjson.go`, `wire/*_test.go`, `wire/fuzz_test.go`

**Produces:** strict envelope/direct-record decoder, validated encoder and bounded recoverable NDJSON framing.

- [x] Add RED tests for duplicate keys, unknown fields, discriminator/payload mismatch, `seq` mismatch, telemetry non-object payload, malformed line recovery and read errors.
- [x] Implement strict decode, outbound self-validation and bounded NDJSON reader/writer.
- [x] Add wire fuzz target.
- [x] Run wire tests to GREEN.

### Task 8: Renderer-neutral layout contract

**Files:** `layout/layout.go`, `layout/layout_test.go`

**Produces:** deterministic border/padding/gap/viewport dimension calculations independent of Bubble Tea.

- [x] Add tests for row/column gap, border/padding budget and viewport clamping.
- [x] Implement box-model calculations.
- [x] Run layout tests to GREEN.

### Task 9: Native/thread-affine actor

**Files:** `external/actor.go`, `external/actor_test.go`

**Produces:** one-OS-thread serialized executor with health lifecycle and failure containment.

- [x] Add RED tests for timeout degradation, queue behavior, close lifecycle and executor panic.
- [x] Implement `LockOSThread`, health states and panic/timeout degradation.
- [x] Run external tests to GREEN.

### Task 10: HTTP streaming / HTTP2 transport

**Files:** `transport/httpstream/httpstream.go`, `transport/httpstream/httpstream_test.go`

**Produces:** ordered NDJSON streaming handler with per-record flush and protocol-valid error behavior.

- [x] Add tests for streaming, real TLS HTTP/2 path, mid-stream errors and invalid first outbound envelope.
- [x] Validate outbound envelopes before committing successful HTTP status.
- [x] Run HTTP transport tests to GREEN.

### Task 11: MCP bridge

**Files:** `transport/mcp/mcp.go`, `transport/mcp/mcp_test.go`

**Produces:** strict JSON-RPC mapping plus `2026-07-28` MCP routing-header helpers.

- [x] Add tests for mapping round-trip, nested A2UI validation and routing header/body consistency.
- [x] Implement strict inbound/outbound envelope validation.
- [x] Run MCP bridge tests to GREEN.

### Task 12: UDP telemetry profile

**Files:** `transport/udp/udp.go`, `transport/udp/udp_test.go`

**Produces:** bounded telemetry-only single-datagram codec.

- [x] Add tests rejecting authoritative kinds, zero sequence, oversize, unknown/duplicate header fields and non-object telemetry payload.
- [x] Reuse common A2UI envelope validation inside the UDP codec.
- [x] Run UDP tests to GREEN.

### Task 13: Schema, conformance and migration documentation

**Files:** `assets/schema.json`, `assets/example-*.ndjson`, `conformance/**`, `references/**`, `SKILL.md`, `README.md`

**Produces:** executable wire contract, record/replay fixture and normative documentation aligned with runtime behavior.

- [x] Connect all component props through discriminated schemas.
- [x] Describe strict hardened envelopes and finite limits.
- [x] Add conformance replay fixture and migration notes.
- [x] Validate shipped examples against Draft 2020-12 schema.

### Task 14: Release verification and artifact

**Files:** `.github/workflows/ci.yml`, generated release archive outside the source tree.

**Produces:** verified source bundle suitable for handoff.

- [x] Run fresh formatting check.
- [x] Run `go test ./...`.
- [x] Run `go vet ./...`.
- [x] Run `go test -race ./...`.
- [x] Run both fuzz smoke gates.
- [x] Revalidate schema examples.
- [x] Scan the source tree for placeholders/temp artifacts.
- [x] Build `a2ui-v1-hardened.zip` and verify it with `unzip -t`.
- [x] Compute SHA-256 for the archive.
