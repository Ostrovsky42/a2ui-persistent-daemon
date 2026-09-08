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
├── localenv/                 local lifecycle descriptor, locks, reconciliation
├── supervisor/               bounded /status polling + viewer lifecycle ownership
├── cmd/
│   ├── a2uid/                persistent semantic daemon
│   ├── a2ui/                 TUI + agent CLI + local `up`/supervisor mode
│   ├── a2ui-mcp/             MCP stdio server backed by the daemon HTTP surface
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

## Persistent daemon / client / local supervisor

Semantic lifetime отделён и от terminal window, и от shell, из которого был запущен runtime. Основной managed path:

```text
                  a2ui up
                     │
          reconcile / reuse / start
             ┌───────┴────────┐
             │                │
             ▼                ▼
           a2uid          supervisor
 Session + Engine +       /status polling
 Document + Runtime       viewer lifecycle
             │                │
             │          pending publication
             │          + no active viewer
             │                │
             │                ▼
             │        terminal emulator
             │                │
             │                ▼
             └────────────── a2ui
                         Bubble Tea TUI
```

`a2uid` остаётся единственным владельцем semantic state. Supervisor не владеет Document, events, publication semantics или agent transport; он только наблюдает authoritative `/status` и управляет desktop viewer lifecycle.

`a2ui up` — короткоживущий reconciler. Он сериализует startup, переиспользует совместимый живой daemon/supervisor или запускает отсутствующий процесс, ждёт readiness и возвращает shell. Daemon и supervisor после этого живут независимо.

После `make build` или установки:

```bash
# managed local environment; default preset=dashboard, viewer policy=auto
a2ui up

# keep daemon/supervisor but never open a viewer automatically
a2ui up --viewer-policy never

# explicit desktop launch policy; still requires a supported terminal backend
a2ui up --viewer-policy always --preset dense
```

Default socket path — `$XDG_RUNTIME_DIR/a2ui/a2ui.sock`; fallback — `/tmp/a2ui-$UID/a2ui.sock`. Runtime metadata живёт рядом с сокетом: `environment.json`, `up.lock`, `supervisor.lock`, diagnostic PID files и private logs. Runtime directory остаётся `0700`; local metadata/log files — private.

Supervisor опрашивает существующий `/status` с фиксированным cadence 250 ms. Request budget отделён от cadence и по умолчанию составляет 2 s; несколько последовательных failed status reads образуют bounded daemon-loss grace. Supervisor привязан к immutable daemon identity (`instance_id`, socket, server, session) и завершается при доказанной замене/несовместимости daemon вместо того, чтобы фабриковать health.

Authoritative viewer fact — существующий interactive lease (`has_client`). Новый pending publication без viewer запускает не detached TUI, а настоящий terminal emulator. Linux discovery order:

```text
xdg-terminal-exec
gnome-terminal
kitty
alacritty
konsole
```

Запуск строится только через argv, без shell-composed command strings. В `auto` режиме viewer spawn подавляется в CI, SSH и headless environment; `never` запрещает auto-spawn явно, `always` обходит эти environment guards, но не отсутствие terminal backend.

Если host launcher возвращает ошибку или viewer не приобретает lease до attach deadline, supervisor завершает процесс с диагностикой и освобождает `supervisor.lock`. Это сознательный recovery boundary: та же generation не ретраится в poll-loop одного supervisor lifetime. Следующий `a2ui up` переиспользует здоровый daemon, запускает новый supervisor, а bootstrap может сделать одну новую попытку для всё ещё pending generation.

Закрытие `a2ui` освобождает только interactive renderer lease; Document, focus, input values, table selection и agent session остаются в daemon. Следующий client получает atomic `Engine.PresentationSnapshot()` и продолжает с текущего состояния. Одновременно допускается только один interactive client.

Низкоуровневый ручной путь остаётся полезен для разработки и диагностики:

```bash
# persistent daemon; optional MCP HTTP bridge
go run ./cmd/a2uid -server 127.0.0.1:8080

# manual attach/detach terminal renderer
go run ./cmd/a2ui -preset dashboard
```

Daemon/client IPC — **local control protocol**, а не новый public A2UI operation set. Он использует bounded strict NDJSON с `hello`, `interaction`, `snapshot`, `frame_published`, `detach` и structured `ipc.*` errors. Snapshot delivery не означает visual publication: pending agent commit подтверждается только после render exact publication generation и `frame_published`. Disconnect before ACK оставляет barrier pending для следующего client.

Важно: `has_client=true` доказывает acquisition renderer lease, но не человеческое внимание или понимание кадра. Manual desktop acceptance реального terminal launch остаётся отдельным host-level доказательством.

`cmd/a2ui-runner` остаётся standalone/in-process инструментом для renderer development, conformance и CI.

## Developer and agent workflow

Run `make help` from the module root for the persistent-daemon developer commands. The reproducible daemon, smoke, focused-test, agent CLI and operating workflow is documented in [docs/agent-kit.md](docs/agent-kit.md).

## Omarchy / protocol / security documentation

Omarchy-oriented documentation разделена по аудиториям вместо дублирования одного большого spec:

- [staged Omarchy Manual chapter](docs/manual/a2ui.md) — user-facing draft;
- [six-operation protocol guide](docs/protocol-guide.md) — practical sequencing и executable `omarchy-choice.ndjson` fixture;
- [security model](docs/security-model.md) — threat boundaries и bounded Bash comparison;
- [security evidence ledger](docs/security-evidence.md) — authoritative claim → implementation → test → status ledger;
- [Omarchy maintainer proposal draft](docs/omarchy-submission.md) — architecture, demo acceptance, packaging/integration and upstream gates;
- [A2UI for Omarchy overview](references/OMARCHY.md) — pitch и links без дублирования normative protocol.

`references/PROTOCOL.md` остаётся normative source для semantic contract. Changing security/release verdicts не дублируются в README: текущие VERIFIED/LIMITED/OPEN claims принадлежат `docs/security-evidence.md`. В частности, terminal-control sanitization имеет regression evidence, тогда как authenticated remote HTTP/MCP ingress не является текущей гарантией.

Документационный пакет reviewable, но не является готовой upstream Omarchy submission. Перед такой подачей нужны final-candidate verification, clean installed end-to-end demo на выбранной версии Omarchy, реальный desktop acceptance, resource measurements и актуальная reproduction evidence.

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
