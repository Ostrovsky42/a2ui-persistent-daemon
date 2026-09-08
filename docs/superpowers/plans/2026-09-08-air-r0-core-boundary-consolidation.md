# AIR R0 Core Boundary Consolidation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove AIR's renderer-independent runtime boundary while removing cheap pre-release public/API debt without expanding the product surface.

**Architecture:** Keep the current transactional Document/Engine/session core. Rename the public identity to AIR/1, make renderer input semantic at daemon IPC, add a deliberately minimal loopback Web renderer as an adversarial consumer of `PresentationSnapshot`, and fix two small core smells without changing external publication semantics.

**Tech Stack:** Go 1.24, net/http, existing daemon/IPC/Engine packages, Bubble Tea adapter, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-08-air-r0-core-boundary-consolidation-design.md`

## Global Constraints

- No new node types or AIR/2.
- No new transport profile; Web is a local renderer surface, not an agent transport.
- No disk persistence, multi-writer, observer/controller protocol, native GUI toolkit, UDP expansion, or native actor expansion.
- Preserve publication-generation/ACK and event-causality contracts.
- Use RED -> GREEN for behavioral changes.
- Keep `a2ui`/`a2uid` command names for R0.
- Do not choose a license without an authoritative repository/user decision.

---

### Task 1: Public Go identity and AIR/1 naming

**Files:**
- Modify: `go.mod`
- Modify: all Go files importing `a2ui/...`
- Modify: `README.md`
- Modify: `references/PROTOCOL.md`
- Modify: `references/IMPLEMENTATION.md`, `references/TRANSPORTS.md`, schemas/examples and non-historical public docs where the current dialect is called A2UI V1

**Interfaces:**
- Produces canonical module path `github.com/Ostrovsky42/agent-interaction-runtime`.
- Produces public protocol name `AIR/1` while retaining JSON `v:1` and six operations.

- [ ] Characterize import surface and public A2UI V1 references.
- [ ] Update `go.mod` and all Go imports atomically.
- [ ] Update product-facing documentation to AIR/1; retain historical provenance where appropriate.
- [ ] Run full format/test/race/vet gates.
- [ ] Commit as a focused identity migration.

### Task 2: Characterize renderer-specific daemon interactions

**Files:**
- Modify/Test: `ipc/protocol_test.go` or focused IPC test file
- Modify/Test: `daemon/*_test.go`
- Inspect: `adapter/bubbletea`, `engine`, `runtime/table_selection.go`, action binding code

**Interfaces:**
- Consumes current `ipc.Interaction` and Engine methods.
- Produces failing tests proving that semantic action invocation/direct row identity must not require a physical key.

- [ ] Add RED test for semantic action invocation without `action_key`.
- [ ] Add RED test for direct row selection/activation by stable `row_id` if required by Web proof.
- [ ] Run focused tests and record expected failures before production changes.

### Task 3: Make daemon interaction IPC semantic

**Files:**
- Modify: `ipc/protocol.go`
- Modify: `daemon/interactions.go`
- Modify: Bubble Tea client/adapter mapping files
- Modify tests from Task 2

**Interfaces:**
- Produces renderer-neutral interaction messages derived from existing Engine semantics.
- Physical keyboard handling remains inside the Bubble Tea side.

- [ ] Implement minimal semantic interaction forms required by the REDs.
- [ ] Preserve or explicitly version local IPC compatibility when a wire break is unavoidable.
- [ ] Run focused IPC/daemon/TUI tests.
- [ ] Run full suite and commit.

### Task 4: Add minimal local Web renderer RED/proof

**Files:**
- Create: focused Web renderer package under `adapter/web` or equivalent renderer location
- Create: minimal command only if needed for manual proof
- Create/Test: renderer tests and daemon attach/detach integration test

**Interfaces:**
- Consumes `Engine.PresentationSnapshot()` / daemon snapshot IPC.
- Produces loopback-only HTML rendering and semantic interaction posts for existing node types needed by the proof.

- [ ] Write failing integration test for TUI detach -> Web attach -> same semantic state.
- [ ] Write failing test for Web semantic action/input/table interaction without keyboard IPC.
- [ ] Implement the smallest loopback Web renderer using Go standard library where practical.
- [ ] Support only text/input/actions/table/progress/box plus viewport if demanded by proof.
- [ ] Do not add styling DSL or new node types.
- [ ] Document any layout leak discovered by the second renderer.
- [ ] Run focused and full tests, then commit.

### Task 5: Separate EventBroker enqueue order from Event.Seq

**Files:**
- Modify: `runtime/events.go`
- Modify/Test: `runtime/events_actions_test.go` and causality regressions

**Interfaces:**
- `protocol.Event.Seq` remains externally visible contiguous delivery sequence only.
- Private queued state carries enqueue ordering used for coalescing.

- [ ] Add characterization test that would fail if internal order is stored in externally meaningful Event.Seq.
- [ ] Verify RED on the intended invariant.
- [ ] Introduce private queued event/order representation.
- [ ] Preserve contiguous delivery sequence and coalescing order.
- [ ] Run causality tests/full suite and commit.

### Task 6: Reject duplicate ActionRegistry registrations

**Files:**
- Modify: `runtime/actions.go`
- Modify/Test: `runtime/events_actions_test.go`

**Interfaces:**
- `Register(id,h)` returns an error on duplicate ID.
- No `Replace` API unless a current call site proves it necessary.

- [ ] Add RED duplicate-registration test.
- [ ] Inspect all Register call sites for intentional replacement.
- [ ] Implement duplicate rejection.
- [ ] Run focused/full tests and commit.

### Task 7: Establish performance baselines

**Files:**
- Create/Modify: benchmark test files in `document`, `engine`, and/or `ipc`
- Create: evidence note under `docs/audits/` if repository convention supports it

**Interfaces:**
- No production optimization.
- Benchmarks cover near-limit apply, large text/table mutation, PresentationSnapshot, and snapshot/attach path where measurable.

- [ ] Add deterministic benchmark fixtures.
- [ ] Run benchmark command in CI-compatible form where possible.
- [ ] Record baseline environment/results without hard thresholds.
- [ ] Commit benchmark-only change.

### Task 8: Add macOS core CI

**Files:**
- Modify: `.github/workflows/ci.yml`

**Interfaces:**
- Linux full gate remains authoritative for fuzz/race matrix.
- macOS adds at least `go test ./...` and `go vet ./...` unless a concrete intentional Linux-only blocker is documented.

- [ ] Add macOS job.
- [ ] Verify workflow on PR current head.
- [ ] Fix platform assumptions only when they violate a claimed platform-neutral boundary.
- [ ] Commit CI change.

### Task 9: Persistence wording and evidence closure

**Files:**
- Modify: `README.md`
- Modify: `docs/security-evidence.md` if boundaries/evidence changed
- Modify: implementation plan checkbox/status/evidence sections as appropriate

**Interfaces:**
- Public promise: daemon-owned, renderer-independent state survives renderer detach/reconnect; no daemon-restart/reboot durability claim.

- [ ] Remove ambiguous disk-durability wording.
- [ ] Record renderer-boundary evidence and any discovered leaks.
- [ ] Confirm no status claim is duplicated into README.
- [ ] Run documentation/link sanity plus full CI.

### Task 10: Draft PR and final acceptance

**Files:** PR metadata only, plus evidence docs if needed.

- [ ] Open Draft PR against current `develop`.
- [ ] Record historical RED evidence and current-head CI policy.
- [ ] Verify diff contains no scope expansion.
- [ ] Run/record manual TUI detach -> Web attach -> semantic interaction proof when a graphical/browser environment is available.
- [ ] Keep Draft if second-renderer proof/current-head CI is incomplete.
- [ ] Final report must list base, branch, head, PR, identity, renderer proof, IPC changes, Event.Seq, action registry, benchmarks, Linux/macOS CI, frozen scope, open gates, and GREEN/RED/PARTIAL verdict.
