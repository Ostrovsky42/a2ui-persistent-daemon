# AIR R0 Core Boundary Consolidation Design

## Status

Approved checkpoint design. Mutable execution status belongs to the implementation plan, PR checks, and evidence documents.

## Goal

Prove that AIR is a renderer-independent, daemon-owned human-interaction runtime rather than a terminal UI framework or a new industry protocol standard.

The durable architectural promise for R0 is deliberately narrow: semantic state survives renderer detach/reconnect while the daemon lives. Disk durability across daemon restart or reboot is not part of this checkpoint.

## Authoritative base

Execution starts from the current remote `develop` after inspection. At design creation time the observed base was `489186fb56a07e8c8d5bf6f111a3bcff0fba7175`; executors must re-check before rebasing or integrating.

Open PRs #6 and #7 contain later agent/supervisor work but are not dependencies of this checkpoint. R0 therefore starts from `develop` and must not silently copy or rebase unmerged P0 work.

## Product boundary

```text
agent ingress
      |
      v
semantic AIR runtime
      |
      +-- authoritative Document
      +-- runtime projection
      +-- publication causality
      +-- reliable human events
      +-- renderer-independent session
      +-- one interactive writer
      |
      v
renderer boundary
      |
   +--+--+
   v     v
  TUI   Web
```

The independent lifetimes are agent process, renderer process/window, human response time, and daemon-owned session.

## Preserved invariants

- `document.Document` remains the sole authoritative semantic UI state.
- Mutations remain transactional: clone candidate, apply, validate invariants, then promote.
- Strict JSON, duplicate-key rejection, finite resource limits, and critical-event backpressure remain intact.
- Publication generation and renderer `frame_published` acknowledgement remain exact-generation causality facts, not proof of human attention.
- Renderer-local state does not create semantic publication.
- A single interactive writer remains the ownership rule in R0.
- Existing protocol wire version remains `v: 1`; the public name changes to AIR/1 without adding a seventh mutation or new node type.

## Scope freeze

R0 adds no new node types, transports, styling system, plugin system, native GUI toolkit, multi-writer collaboration, observer/controller protocol, disk WAL/database, remote HTTP exposure, UDP feature, native/CGO actor feature, or second internal protocol version.

Existing UDP and native/external code is preserved but frozen.

## Public identity cleanup

The Go module becomes `github.com/Ostrovsky42/agent-interaction-runtime`. Public documentation calls the current six-operation dialect `AIR/1`. Historical provenance documents may retain A2UI terminology when rewriting them would destroy context. Existing `a2ui`/`a2uid` command names remain unchanged in R0.

A repository license is required before external adoption, but the implementation must not choose MIT, Apache-2.0, or another license without an authoritative user decision.

## Renderer interaction boundary

Physical input is renderer-local. In particular, a keyboard key must not be the canonical daemon interaction API. The second renderer must be able to express meaningful user actions semantically.

The implementation should derive the smallest interaction vocabulary from existing Engine operations. Likely semantic forms include focus, input set/submit, action invocation, and direct row selection/activation. Relative table movement is not presumed wrong; it is retained or changed only based on the Web renderer proof.

## Second renderer proof

A minimal local Web renderer is the adversarial test. It is not a product UI and must bind loopback-only by default. It reuses the same `Engine.PresentationSnapshot()` semantics and existing daemon ownership.

Required proof:

```text
TUI attach -> render -> detach
       daemon/session remains
Web attach -> same semantic state -> human interaction -> semantic event
```

No simultaneous TUI/Web observation is required. If sequential replacement cannot work without changing ownership semantics, that is an architectural RED to document rather than a reason to add speculative observer infrastructure.

The Web renderer must expose existing assumptions around padding, gap, border, and rounded layout rather than redesigning them in advance.

## Small core debt

### Event ordering

`protocol.Event.Seq` must have one meaning: externally visible contiguous delivery sequence. Internal enqueue order moves to private broker state, such as `queuedEvent{order,event}`. Existing coalescing order and causality tests must remain valid.

### Action registration

`ActionRegistry.Register` must reject duplicate capability IDs instead of silently replacing handlers. No speculative `Replace` API is added unless an existing production call site requires it.

## Performance fence

Keep the correctness-first reducer. Add reproducible benchmarks for near-limit document mutation, large text/table cases, `PresentationSnapshot`, and attach/snapshot work where practical. R0 establishes baselines; it does not optimize without evidence.

## Platform verification

Linux remains primary. CI should add a lightweight macOS core job (`go test ./...`, `go vet ./...`) unless a concrete platform-specific package blocks it; such a blocker must be documented rather than hidden by disabling meaningful tests.

## Persistence wording

Public docs must say that state is long-lived, daemon-owned, and renderer-independent. They must explicitly avoid promising survival across daemon restart/reboot.

## Deferred items

- fat `protocol.Event` wire redesign / discriminated payloads;
- signedness cleanup for sequencing unless required by a discovered invariant;
- Google A2UI or MCP Apps compatibility adapter;
- disk durability and exactly-once recovery after daemon restart;
- multi-observer/controller ownership.

## Acceptance

R0 is GREEN only when public identity is consistent, keyboard-specific daemon IPC is removed from the canonical interaction path, the minimal Web renderer proves sequential TUI-to-Web replacement on the same daemon-owned state, Event.Seq has one semantic meaning, duplicate action registration is rejected, benchmark baselines exist, Linux CI stays green, macOS core verification exists or has a documented blocker, and the scope freeze is preserved.