# P0.2 R0 — render visibility acknowledgement characterization

Status: characterization only. No production semantics changed by this checkpoint.

Base branch head at characterization start:

`49427a847a10052753bd06d3cfc56254d29b221d`

That P0.1 head passed the repository CI matrix (build, unit/conformance, race, vet, and fuzz smoke). Real Codex + real human acceptance remains a separate open gate.

## Question

Can the current `frame_published` / `committed` path honestly be used as the terminal-visible point for P0.2 render lifecycle telemetry?

## Current contract boundary

The engine contract says publication acknowledgement belongs after the renderer has made the current snapshot visible. `Engine.PublishGeneration(generation)` is therefore the semantic barrier that releases pending `committed` events.

The daemon accepts an IPC `frame_published` message carrying `publication_generation`, validates that it matches the current generation, and then calls `Engine.PublishGeneration`.

## Observed implementation order

The current Bubble Tea adapter does this inside `Model.View()`:

1. read one semantic `PresentationSnapshot`;
2. render the snapshot to a string;
3. while still inside `View()`, call `AcknowledgePublication(snapshot.PublicationGeneration)`;
4. return the rendered string to Bubble Tea.

For the remote controller, `AcknowledgePublication` immediately writes the IPC `frame_published` message to the daemon.

The repository pins Bubble Tea `v1.3.10`. Its standard renderer does not synchronously write the string returned by `View()` to the terminal. `standardRenderer.write(s)` replaces an internal buffer, and a ticker later calls `flush()`. `flush()` performs the actual `r.out.Write(...)`. Multiple views can therefore be coalesced before a terminal write.

The effective ordering is currently:

```text
snapshot G
  -> A2UI RenderFrame(G)
  -> frame_published(G) sent from Model.View()
  -> daemon PublishGeneration(G)
  -> committed event may become visible to the agent
  -> Model.View() returns string
  -> Bubble Tea queues latest string
  -> later renderer ticker flushes output to terminal
```

## Finding

`frame_published` currently means "the A2UI adapter rendered a view candidate and attempted the acknowledgement", not "the terminal output write for that generation completed".

Therefore P0.2 MUST NOT label the existing ACK as:

- `terminal_write_complete`;
- `actual_paint`;
- an exact publish-to-terminal-visible latency.

This is not just a naming problem for metrics. `committed` may currently be released before Bubble Tea has written the corresponding frame bytes to the terminal output.

## What is measurable without changing the renderer boundary

The following measurements are honest with the current architecture:

### MCP process

- tool call start/end;
- publish request duration;
- wait-event return duration.

### Daemon

- mutation receive/apply duration;
- publication generation creation;
- snapshot-ready time;
- time until current `frame_published` ACK is observed;
- document node/byte counts;
- event queue depth and event delivery durations.

### Bubble Tea adapter

- snapshot read/receive time;
- `RenderFrame` duration;
- `View()` completion;
- local interaction processing duration.

### Terminal output wrapper

A writer supplied through `tea.WithOutput(...)` can measure generic write count, byte count and `Write` duration. However it cannot, with the current public Bubble Tea renderer API, safely prove which `publication_generation` produced a given flush.

## Why a simple `WithOutput` wrapper is insufficient for exact correlation

Bubble Tea `v1.3.10` uses a framerate-based renderer:

```text
View A -> renderer buffer A
View B -> renderer buffer B (A can be replaced)
ticker -> flush latest buffer
```

A shared "latest generation" atomic in an output wrapper is not a proof. There is a race between `View()` updating that generation and Bubble Tea replacing/flushing its private renderer buffer. A flush of an older buffer could be attributed to a newer generation.

Treating this as exact correlation would create observability data stronger than the implementation can justify.

## Options

### Option A — weaken the publication meaning

Define `frame_published` as "renderer accepted a view candidate" and instrument that boundary only.

Advantages:

- smallest change;
- current implementation already behaves this way.

Costs:

- weakens the existing publication-barrier contract;
- `committed` can precede terminal output;
- does not satisfy the intended P0.2 visibility measurement.

Recommendation: reject.

### Option B — best-effort terminal writer correlation

Wrap `tea.WithOutput`, associate writes with the latest observed generation, and call the data approximate.

Advantages:

- low implementation cost;
- useful generic terminal write timing.

Costs:

- generation attribution is racy under renderer coalescing;
- cannot be used to move the semantic publication ACK;
- risks turning approximate telemetry into a future hidden invariant.

Recommendation: acceptable only for uncorrelated writer counters, not for the publication lifecycle.

### Option C — introduce an explicit generation-aware terminal visibility boundary

Move publication acknowledgement out of `Model.View()` and make the component that owns successful terminal output acknowledge the exact generation it wrote.

This requires one of two implementation strategies:

1. an A2UI-owned terminal frame sink/render loop, while Bubble Tea continues to own input/update orchestration; or
2. a renderer hook/fork that exposes the exact generation/frame at Bubble Tea flush completion.

Advantages:

- restores the intended publication-barrier semantics;
- gives P0.2 an exact `publish -> output write -> ACK` correlation point;
- superseded generations become observable rather than ambiguous.

Costs:

- this is a renderer-boundary change, not a metrics-only patch;
- must preserve alt-screen, redraw, reconnect and animation behavior;
- must not make telemetry a prerequisite for semantic correctness.

Recommendation: pursue Option C, but characterize the smallest safe renderer integration before production changes.

## Proposed checkpoint split

### P0.2-R0 — VISIBILITY ACK CHARACTERIZATION

This document. No production changes.

### P0.2-R1 — POST-WRITE ACK PROOF

Goal: establish an exact, testable point after terminal output write completion for publication generation `G`.

Required invariants:

```text
ACK(G) cannot happen before successful output write for G
stale/superseded G cannot ACK newer generation
write failure cannot emit successful ACK
reconnect preserves pending generation
telemetry failure cannot block or manufacture ACK
```

No A2UI V1 change.

### P0.2-R2 — PUBLICATION TRACE

Once R1 exists, record a bounded in-memory trace keyed by publication generation + frame:

```text
MCP publish
-> daemon apply
-> publication generation
-> client snapshot receive
-> render start/end
-> terminal write start/end
-> daemon ACK observed
```

Durations are measured only by monotonic clocks local to each process. Cross-process wall-clock subtraction is forbidden.

### P0.2-R3 — HUMAN EVENT RETURN TRACE

Correlate:

```text
interaction
-> semantic event enqueue
-> broker delivery
-> a2ui_wait_event
-> MCP return
```

`source_generation` may initially be observability metadata only. Stale-event rejection belongs to the later interaction-epoch checkpoint, not R3.

## Metrics after R1/R2

Only after an exact visibility boundary exists should the system expose per-generation values such as:

- `publish_to_frame_ack`;
- `render_duration`;
- `terminal_write_duration`;
- `document_nodes` / `document_bytes`;
- `event_queue_depth`;
- `publication_superseded_total`;
- `interaction_to_mcp_return`.

The term `actual_paint` remains prohibited: a successful write to the terminal file descriptor does not prove that a terminal emulator has physically painted pixels.

## Stop condition

Do not implement render lifecycle telemetry that claims exact terminal visibility until P0.2-R1 has a generation-aware post-write ACK boundary.

Do not change A2UI Protocol V1 to solve this. The problem is inside the local renderer/IPC boundary.
