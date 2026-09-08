# Agent Interaction Runtime

A local persistent human-interaction runtime for AI agents.

> **Historical naming.** This repository was originally developed under the name A2UI. Its current `AIR/1` wire dialect predates and is not an implementation of Google's [A2UI specification](https://github.com/google/A2UI). The wire models are different and currently not compatible. External A2UI compatibility is an explicitly open architectural question described in [Direction](#direction).

AIR separates the lifetime of an agent session from the lifetime of the surface a human happens to be looking at.

An agent describes *what it needs from a human* as declarative semantic UI. A long-lived local daemon owns that state. A user-facing surface appears, the human interacts, the surface goes away — and the agent's session survives all of it.

```text
agent publishes intent
      ↓
state survives
      ↓
human surface appears
      ↓
human interacts
      ↓
surface disappears
      ↓
session remains
      ↓
next interaction resumes correctly
```

Four lifetimes are deliberately separated:

```text
agent lifetime        the harness/process that published
renderer lifetime     the window the human is looking at
human-response time   however long a person takes
session lifetime      owned by the daemon
```

Binding these into one process is the architectural mistake this project exists to avoid.

**What this is not.** AIR is not a TUI framework. Bubble Tea, Ratatui, Textual and Ink already solve terminal rendering well; the Bubble Tea adapter here consumes that layer rather than competing with it. The engineering weight sits in the runtime: authoritative state, transactional mutation, publication generations, renderer acknowledgement, reconnect, interactive lease, event causality and process ownership.

The current `AIR/1` protocol is one semantic dialect used by AIR, not the definition of the project. NDJSON, HTTP streaming, MCP and UDP telemetry remain transport profiles around that semantic core.

```text
               AIR interaction runtime
                        │
                 AIR/1 dialect
                        │
       ┌────────────────┼────────────────┐
       │                │                │
  stdio / NDJSON    HTTP streaming    MCP bridge
       │                │                │
       └──────────── reliable ───────────┘
                        │
                   Session layer
                        │
             validate → candidate
                        │
             transactional Document
                        │
             runtime-local projection
                        │
                renderer adapter

UDP telemetry ────── lossy side profile only
```

## What is core now

- `document.Document` is the single authoritative UI state.
- A failed mutation does not change the Document.
- `upsert` = full replacement props; `props` = shallow top-level merge.
- `box` and `viewport` may have children; all other node types are leaves.
- Focus, input value and semantic table selection are runtime projection; input caret, viewport offset, animation phase and terminal dimensions are renderer/adapter-local presentation state.
- `input.force` is a one-shot mutation policy and is not retained in props.
- Critical events cannot silently disappear under backpressure.
- Action bindings are deterministic in tree order; a conflicting key rejects the candidate.
- `commit` is a publication barrier carrying `revision` and `through_seq`, not a synthetic dirty flag.
- All incoming and retained Document resources have finite limits, including aggregate `max_document_bytes`.
- Unknown fields/props and duplicate JSON keys are rejected.

## Packages

```text
agent-interaction-runtime/
├── protocol/                 current AIR/1 wire dialect, validation, limits, errors
├── document/                 authoritative tree + transactional reducer
├── runtime/                  focus, input-local state, bindings, events, actions
├── engine/                   Document + runtime + event broker integration
├── session/                  hello/capabilities + reliable sequencing
├── wire/                     strict JSON + bounded NDJSON codec
├── layout/                   renderer-neutral box model + deterministic flex allocator
├── external/                 thread-affine actor for CGO/native APIs
├── ipc/                      strict local daemon/client NDJSON control protocol
├── daemon/                   persistent Engine + Session owner and IPC server
├── cmd/
│   ├── a2uid/                persistent daemon
│   ├── a2ui/                 thin Bubble Tea IPC client
│   └── a2ui-runner/          standalone dev/showcase runner
├── transport/
│   ├── httpstream/           ordered streaming handler; HTTP/2-capable over Go TLS
│   ├── mcp/                  JSON-RPC/MCP 2026-07-28 bridge helpers
│   └── udp/                  telemetry-only single-datagram profile
├── conformance/              replay fixtures
├── reference-impl/           small buildable facade over the verified core
├── assets/                   JSON Schema and wire examples
└── references/               normative protocol and implementation notes
```

Core deliberately uses only the Go standard library. Renderer dependencies are adapters and do not participate in protocol correctness.

## Quick start

```go
package main

import (
    "encoding/json"

    "github.com/Ostrovsky42/agent-interaction-runtime/engine"
    "github.com/Ostrovsky42/agent-interaction-runtime/protocol"
)

func main() {
    limits := protocol.DefaultLimits()
    e := engine.New(limits, limits.MaxPendingEvents, engine.NewNoopActions())

    err := e.Apply(protocol.Operation{
        V:      1,
        Seq:    1,
        Op:     protocol.OpUpsert,
        ID:     "main",
        Type:   protocol.NodeBox,
        Parent: "root",
        Props:  json.RawMessage(`{"dir":"col","gap":1}`),
    })
    if err != nil {
        panic(err)
    }
}
```

## Bubble Tea Renderer V2

`adapter/bubbletea` is the first full expressive renderer. Semantic props remain in the Document; concrete visualization is selected by a renderer-side preset:

```text
minimal    calm tool/assistant console
dashboard  panels/cards/status hierarchy
dense      high-density admin/developer view
```

A preset is **not a protocol property**. The same Document can be rendered by all three:

```bash
go run ./cmd/a2ui-runner -scenario showcase -preset minimal -width 120
go run ./cmd/a2ui-runner -scenario showcase -preset dashboard -width 120
go run ./cmd/a2ui-runner -scenario showcase -preset dense -width 80
```

Renderer V2 supports semantic variants (`title`, `card`, `toolbar`, `spinner`, `dense`, etc.), renderer-neutral `flex` hints, `responsive:"stack"`, viewport clipping/follow-tail and event-driven spinner/cursor animation. Animation phase, cursor blink and terminal size are never written into `document.Document`. There is no permanent idle tick loop.

The terminal renderer is the first surface, not the definition of AIR.

## Interactive Runtime V3

V3 adds bidirectional terminal interaction on top of the same semantic core:

```text
Document            agent-owned semantic UI
Runtime projection  focus + input value + table selection
Adapter local       input caret + viewport offset/pin + animation + terminal size
```

Selectable tables support stable `row_ids` and optional `action`; selection survives agent reorder by row ID, while `Enter` creates a reliable critical `select` event with `row`, `row_id` and `action`. A scrollable viewport (`scrollable:true`) participates in deterministic Tab order and supports `Up/Down`, `PageUp/PageDown`, `Home/End`; manual scroll removes the `follow_tail` pin, and `End` restores it. Input editing is rune-safe: `Left/Right`, `Home/End`, `Backspace/Delete`, insertion at caret and `Enter` submit.

Local interaction redraw is separate from protocol publication: selection/caret/scroll do not create fake `commit`/`committed` state. The renderer consumes one atomic `Engine.PresentationSnapshot()` rather than assembling a frame from multiple inconsistent state reads.

Interactive showcase:

```bash
go run ./cmd/a2ui-runner -scenario interactive -preset dashboard -tui
go run ./cmd/a2ui-runner -scenario interactive -preset minimal -tui
go run ./cmd/a2ui-runner -scenario interactive -preset dense -tui
```

## Persistent daemon / client

The persistent runtime separates semantic lifetime from terminal-window lifetime:

```text
Agent / MCP / AIR/1 envelopes
            │
            ▼
         a2uid
   Session + Engine + Document
   Runtime + Events + Actions
            │
      Unix domain socket
            │
            ▼
          a2ui
   Bubble Tea + local caret/scroll
```

`a2uid` is the sole owner of semantic state. Closing `a2ui` releases only the interactive renderer lease; Document, focus, input values, table selection and the agent session remain alive. The next client receives one atomic `Engine.PresentationSnapshot()` and resumes from the current state. Only one interactive client is accepted at a time.

Default socket path is `$XDG_RUNTIME_DIR/a2ui/a2ui.sock`; fallback is `/tmp/a2ui-$UID/a2ui.sock`. The runtime directory is `0700`, socket and startup lock are `0600`. Startup is serialized so concurrent daemons cannot unlink each other while recovering a stale socket.

```bash
# persistent daemon; optional MCP HTTP bridge
go run ./cmd/a2uid -server 127.0.0.1:8080

# attach/detach terminal renderer
go run ./cmd/a2ui -preset dashboard
```

Daemon/client IPC is a **local control protocol**, not a new public A2UI operation set. It uses bounded strict NDJSON with `hello`, `interaction`, `snapshot`, `frame_published`, `detach` and structured `ipc.*` errors. Snapshot delivery never implies visual publication: a pending agent commit is acknowledged only after the client renders that exact publication generation and sends `frame_published`. Disconnect before ACK leaves the barrier pending for the next client.

`cmd/a2ui-runner` remains standalone and in-process for renderer development, conformance and CI.

## Developer and agent workflow

Run `make help` from the module root for the persistent-daemon developer commands. The reproducible daemon, smoke, focused-test and agent operating workflow is documented in [docs/agent-kit.md](docs/agent-kit.md).

## Omarchy / protocol / security documentation

The current Omarchy-oriented documentation checkpoint is split by audience instead of duplicating one large spec:

- [staged Omarchy Manual chapter](docs/manual/a2ui.md) — user-facing draft;
- [six-operation protocol guide](docs/protocol-guide.md) — practical sequencing and the executable `omarchy-choice.ndjson` fixture;
- [security model](docs/security-model.md) — threat boundaries and bounded Bash comparison;
- [security evidence ledger](docs/security-evidence.md) — claim → implementation → test → current status;
- [Omarchy maintainer proposal draft](docs/omarchy-submission.md) — demo, packaging and upstream gates;
- [A2UI for Omarchy overview](references/OMARCHY.md) — historical protocol-facing pitch and links without duplicating the normative protocol.

`references/PROTOCOL.md` remains normative for the current AIR/1 dialect. Mutable security/release status belongs in `docs/security-evidence.md`, not in this README.

## Transport profiles

### NDJSON / stdio

Canonical debug and record/replay format. One JSON value per line. Record size is bounded; malformed lines are recoverable because line framing is explicit.

### HTTP streaming / HTTP/2

`transport/httpstream` is a streaming `net/http.Handler`. It writes and flushes each accepted response record immediately. When hosted by a Go TLS server with HTTP/2 enabled, the same handler operates over HTTP/2 without changing AIR's semantic-state guarantees.

### MCP bridge

`transport/mcp` maps the current AIR/1 envelopes to JSON-RPC messages and targets the modern stateless MCP transport model (`MCP-Protocol-Version: 2026-07-28`). AIR state remains explicit in the current `session` handle; it is not hidden in an MCP transport session.

This package is an integration bridge, **not a claim that `a2ui/*` is an officially registered MCP extension namespace**. A production MCP host can expose the bridge through its extension/tool policy.

[MCP Apps](https://apps.extensions.modelcontextprotocol.io/) covers adjacent ground with UI hosted inside the agent client. AIR's current renderer model is deliberately independent of the host application; compatibility or bridging between those models is an integration question rather than a semantic-core requirement.

### UDP

UDP v1 is intentionally restricted to telemetry. Document mutations, submit/action traffic and commit barriers are rejected by the codec. One A2UI payload = one UDP datagram; no fragmentation/reassembly is implemented. Default datagram budget is 1200 bytes.

## Direction

This section describes **intent, not implemented features.** Everything currently implemented is described above and gated by the repository checks in [Verification](#verification). Nothing in this section carries a release or verification status claim; mutable status belongs in `docs/security-evidence.md`.

### Renderer neutrality is the real test

The semantic boundary the codebase is designed to preserve:

```text
semantic          table, input, text, actions, viewport, box
renderer-local    ANSI attributes, terminal width, caret, alternate screen,
                  window geometry, animation phase
```

If a second renderer can consume `Engine.PresentationSnapshot()` without changing the semantic state machine, that boundary is real. If it cannot, the boundary was decorative.

A local web renderer is the intended cheapest second-surface experiment because it can cover Linux, macOS and Windows without committing AIR to a native GUI toolkit. Native desktop renderers such as SwiftUI, WinUI, GTK or Qt are deliberately out of scope until that architectural test has been run.

### Protocol compatibility is an open question

The six-operation protocol in `references/PROTOCOL.md` is the current canonical AIR dialect. It is not a bid for an industry standard.

Google's A2UI specification covers adjacent ground with a different wire model and a component-catalog approach. MCP Apps covers adjacent ground again, with interactive HTML hosted inside the agent's client rather than as an independent local surface.

Three outcomes remain possible and none is decided:

| | |
|---|---|
| **A** | make AIR a conformant runtime for an external UI specification |
| **B** | keep the AIR core and add an ingress adapter for that specification |
| **C** | keep AIR/1 as an internal dialect and retire the historical protocol name from the public product surface |

B is currently the most natural architectural hypothesis because an ingress adapter is small relative to the runtime it feeds, but this is a direction rather than a commitment.

Whichever path is chosen, the daemon, lifetime separation, publication semantics and causality work remain independent of the external wire format. That runtime layer is the durable part of the project.

### Deliberate non-goals

- a second internal protocol version merely to chase an external specification;
- component-catalog expansion before a real renderer/compatibility requirement exists;
- native GUI toolkits before the second-renderer test;
- rewriting the core in another language without a measured systems reason;
- competing with external UI specifications for standard ownership;
- becoming a general-purpose GUI framework.

## Verification

Repository gates:

```bash
gofmt -w $(find . -name '*.go')
go test ./...
go test -race ./...
go vet ./...
```

Conformance replay lives in `conformance/testdata/` plus the staged Omarchy walkthrough under `assets/examples/`. Fuzz entrypoints are in `wire/fuzz_test.go`, `document/model_test.go`, and `ipc/codec_test.go`.

## Normative documents

1. `references/PROTOCOL.md` — current AIR/1 semantic contract.
2. `assets/schema.json` — executable wire shape for upsert/envelopes/events.
3. `references/TRANSPORTS.md` — transport guarantees and profile restrictions.
4. `references/IMPLEMENTATION.md` — Go package boundaries and extension rules.
5. `SKILL.md` — compressed rules for an LLM agent; it is subordinate to `PROTOCOL.md`.

Design/implementation rationale is retained in `docs/superpowers/specs/` and `docs/superpowers/plans/`.
