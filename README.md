# A2UI v1 Hardened

A2UI — небольшой state-synchronization protocol для интерфейсов, которыми управляет LLM/agent, а исполняет локальный доверенный runtime.

Главное изменение относительно первоначального эскиза: **протокол больше не равен NDJSON и не зависит от Bubble Tea**. NDJSON, HTTP/2, MCP bridge и UDP telemetry — транспортные профили поверх одного semantic core.

```text
                 A2UI semantic protocol
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

UDP telemetry ─────── lossy side profile only
```

## Что теперь является ядром

- `document.Document` — единственный authoritative UI state.
- Failed mutation не меняет Document.
- `upsert` = full replacement props; `props` = shallow top-level merge.
- `box` и `viewport` могут иметь children; остальные node types — leaves.
- Focus, input value и semantic table selection — runtime projection; input caret, viewport offset, animation phase и terminal dimensions — renderer/adapter-local presentation state.
- `input.force` — one-shot mutation policy и не сохраняется в props.
- Critical events не могут молча исчезнуть при backpressure.
- Action bindings детерминированы tree order; конфликт key отклоняет candidate.
- `commit` — publication barrier с `revision` и `through_seq`, а не фиктивный dirty flag.
- Все входные и retained Document ресурсы имеют finite limits, включая aggregate `max_document_bytes`.
- Unknown fields/props и duplicate JSON keys отвергаются.

## Пакеты

```text
a2ui/
├── protocol/                 wire types, props validation, limits, errors
├── document/                 authoritative tree + transactional reducer
├── runtime/                  focus, input-local state, bindings, events, actions
├── engine/                   интеграция Document + runtime + event broker
├── session/                  hello/capabilities + reliable sequencing
├── wire/                     strict JSON + bounded NDJSON codec
├── layout/                   renderer-neutral box model + deterministic flex allocator
├── external/                 thread-affine actor для CGO/native APIs
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
├── reference-impl/           маленький buildable facade над verified core
├── assets/                   JSON Schema и wire examples
└── references/               нормативная спецификация и implementation notes
```

Core deliberately uses only the Go standard library. Renderer dependencies are adapters and do not participate in protocol correctness.

## Быстрый старт

```go
package main

import (
    "encoding/json"

    "a2ui/engine"
    "a2ui/protocol"
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

`adapter/bubbletea` — первый полноценный expressive renderer. Semantic props остаются в Document, а конкретная визуализация выбирается renderer-side preset’ом:

```text
minimal    спокойная tool/assistant console
dashboard  panels/cards/status hierarchy
dense      admin/developer view с высокой плотностью
```

Preset **не является protocol property**. Один и тот же Document можно отрисовать всеми тремя способами:

```bash
go run ./cmd/a2ui-runner -scenario showcase -preset minimal -width 120
go run ./cmd/a2ui-runner -scenario showcase -preset dashboard -width 120
go run ./cmd/a2ui-runner -scenario showcase -preset dense -width 80
```

Renderer V2 поддерживает semantic variants (`title`, `card`, `toolbar`, `spinner`, `dense` и др.), renderer-neutral `flex` hints, `responsive:"stack"`, viewport clipping/follow-tail и event-driven spinner/cursor animation. Animation phase, cursor blink и terminal size никогда не записываются в `document.Document`. В idle-состоянии постоянного tick loop нет.

## Interactive Runtime V3

V3 добавляет двустороннюю terminal interaction поверх того же semantic core:

```text
Document            agent-owned semantic UI
Runtime projection  focus + input value + table selection
Adapter local       input caret + viewport offset/pin + animation + terminal size
```

Selectable tables поддерживают stable `row_ids` и optional `action`; selection переживает agent reorder по row ID, а `Enter` создаёт reliable critical `select` event с `row`, `row_id` и `action`. Scrollable viewport (`scrollable:true`) входит в deterministic Tab-order и поддерживает `Up/Down`, `PageUp/PageDown`, `Home/End`; ручной scroll снимает `follow_tail` pin, а `End` возвращает его. Input editing rune-safe: `Left/Right`, `Home/End`, `Backspace/Delete`, вставка в caret и `Enter` submit.

Local interaction redraw отделён от protocol publication: selection/caret/scroll не создают fake `commit`/`committed`. Renderer получает один atomic `Engine.PresentationSnapshot()` и не собирает кадр из нескольких несогласованных чтений state.

Интерактивный showcase:

```bash
go run ./cmd/a2ui-runner -scenario interactive -preset dashboard -tui
go run ./cmd/a2ui-runner -scenario interactive -preset minimal -tui
go run ./cmd/a2ui-runner -scenario interactive -preset dense -tui
```

## Persistent daemon / client

System-integration Epic 1 separates semantic lifetime from the terminal window:

```text
Agent / MCP / A2UI envelopes
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

Run `make help` from the module root for the persistent-daemon developer commands. The reproducible daemon, smoke, focused-test, and agent operating workflow is documented in [docs/agent-kit.md](docs/agent-kit.md).

## Omarchy / protocol / security documentation

The current Omarchy-oriented documentation checkpoint is split by audience instead of duplicating one large spec:

- [staged Omarchy Manual chapter](docs/manual/a2ui.md) — user-facing draft; explicitly blocked on packaging and a no-JSON agent CLI;
- [six-operation protocol guide](docs/protocol-guide.md) — practical sequencing and the executable `omarchy-choice.ndjson` fixture;
- [security model](docs/security-model.md) — threat boundaries and bounded Bash comparison;
- [security evidence ledger](docs/security-evidence.md) — claim → implementation → test → current status;
- [Omarchy maintainer proposal draft](docs/omarchy-submission.md) — demo, packaging and upstream gates;
- [A2UI for Omarchy overview](references/OMARCHY.md) — pitch and links without duplicating the normative protocol.

`references/PROTOCOL.md` remains normative. The documentation deliberately does **not** claim HTTP caller authentication or terminal escape sanitization until those boundaries have implementation evidence.

The documentation package is reviewable now, but it is not yet an upstream-ready Omarchy submission. The remaining product prerequisites are deliberately visible in the Manual and submission draft instead of being represented as existing features.

## Transport profiles

### NDJSON / stdio

Canonical debug and record/replay format. One JSON value per line. Record size is bounded; malformed lines are recoverable because line framing is explicit.

### HTTP streaming / HTTP/2

`transport/httpstream` is a streaming `net/http.Handler`. It writes and flushes each accepted response record immediately. When hosted by a Go TLS server with HTTP/2 enabled, the same handler operates over HTTP/2 without changing A2UI semantics.

### MCP bridge

`transport/mcp` maps A2UI envelopes to JSON-RPC messages and targets the modern stateless MCP transport model (`MCP-Protocol-Version: 2026-07-28`). A2UI state remains explicit in the A2UI `session` handle; it is not hidden in an MCP transport session.

This package is an integration bridge, **not a claim that `a2ui/*` is an officially registered MCP extension namespace**. A production MCP host can expose the bridge through its extension/tool policy.

### UDP

UDP v1 is intentionally restricted to telemetry. Document mutations, submit/action traffic and commit barriers are rejected by the codec. One A2UI payload = one UDP datagram; no fragmentation/reassembly is implemented. Default datagram budget is 1200 bytes.

## Проверка

Repository gates:

```bash
gofmt -w $(find . -name '*.go')
go test ./...
go test -race ./...
go vet ./...
```

Conformance replay lives in `conformance/testdata/` plus the staged Omarchy walkthrough under `assets/examples/`. Fuzz entrypoints are in `wire/fuzz_test.go`, `document/model_test.go`, and `ipc/codec_test.go`.

## Нормативные документы

1. `references/PROTOCOL.md` — semantic contract.
2. `assets/schema.json` — executable wire shape for upsert/envelopes/events.
3. `references/TRANSPORTS.md` — transport guarantees and profile restrictions.
4. `references/IMPLEMENTATION.md` — Go package boundaries and extension rules.
5. `SKILL.md` — compressed rules for an LLM agent; it is subordinate to `PROTOCOL.md`.

Design/implementation rationale is retained in `docs/superpowers/specs/` and `docs/superpowers/plans/`.
