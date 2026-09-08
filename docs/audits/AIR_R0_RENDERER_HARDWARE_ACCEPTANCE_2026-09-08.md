# AIR R0 renderer hardware acceptance

Date: 2026-09-08

Status: **READY FOR USER HARDWARE — NOT YET ACCEPTED**

Branch: `experiment/20260908-air-r0-web-renderer`

This acceptance is intentionally sequential. AIR still permits one interactive renderer lease at a time.

The proof target is:

```text
same daemon
  -> initial agent publication
  -> TUI renders and ACKs exact publication
  -> human mutates runtime state
  -> TUI detaches
  -> agent mutates semantic Document while no renderer is attached
  -> Web renderer attaches to the same daemon
  -> browser visibility ACKs the pending publication
  -> Web sees both the detached agent update and retained human runtime state
  -> Web sends a semantic response
  -> Web detaches
  -> TUI can reattach to the same state
```

## 1. Prepare the exact candidate

```bash
git fetch origin
git switch experiment/20260908-air-r0-web-renderer
git pull --ff-only

git rev-parse HEAD

go test ./...
go test -race ./...
go vet ./...

rm -rf /tmp/air-r0-hw
mkdir -p /tmp/air-r0-hw/bin /tmp/air-r0-hw/runtime
chmod 700 /tmp/air-r0-hw/runtime

go build -o /tmp/air-r0-hw/bin/a2uid ./cmd/a2uid
go build -o /tmp/air-r0-hw/bin/a2ui ./cmd/a2ui
```

Record the exact tested HEAD in the result section at the bottom. Do not reuse a SHA from this document as an acceptance pin.

Constants used below:

```bash
SOCK=/tmp/air-r0-hw/runtime/air.sock
SERVER=http://127.0.0.1:18080
SESSION=hardware-smoke
```

Use the same values in every shell.

## 2. Start one daemon

Terminal A:

```bash
/tmp/air-r0-hw/bin/a2uid \
  -socket "$SOCK" \
  -server 127.0.0.1:18080 \
  -session "$SESSION"
```

Expected:

```text
a2uid: ipc=/tmp/air-r0-hw/runtime/air.sock mcp=http://127.0.0.1:18080
```

Keep this daemon running for the entire acceptance. Restarting it invalidates the continuity proof.

## 3. Publish the initial semantic surface

Terminal B:

```bash
/tmp/air-r0-hw/bin/a2ui send \
  -server "$SERVER" \
  -session "$SESSION" \
  examples/air-r0-renderer-smoke-initial.ndjson
```

Optional status evidence:

```bash
/tmp/air-r0-hw/bin/a2ui status -server "$SERVER" -json
```

At this point the publication must remain pending because no renderer has made it visible yet.

## 4. Direction A — Bubble Tea TUI

Before launching the TUI, Terminal C:

```bash
/tmp/air-r0-hw/bin/a2ui wait-event \
  -server "$SERVER" \
  -session "$SESSION" \
  -timeout 60s \
  -type committed \
  -raw
```

Terminal B:

```bash
/tmp/air-r0-hw/bin/a2ui -socket "$SOCK" -preset dashboard
```

Expected visible semantic content:

- `AIR R0 hardware smoke — TUI phase`
- one input field
- table rows `Terminal / Bubble Tea` and `Local Web`
- progress at approximately 42%

Expected publication evidence: the waiting `committed` command returns frame `air-r0-tui-phase` only after the TUI has rendered the publication.

### TUI human-response test

Terminal C:

```bash
/tmp/air-r0-hw/bin/a2ui wait-event \
  -server "$SERVER" \
  -session "$SESSION" \
  -timeout 60s \
  -type submit \
  -raw
```

In the TUI:

1. Use `Tab` until the input is focused if necessary.
2. Type `from-tui`.
3. Press `Enter`.

PASS: Terminal C receives a `submit` event with:

```text
id = answer
value = from-tui
action = submit_smoke
```

Optional table event:

1. Start another `wait-event -type select -raw`.
2. `Tab` to the table.
3. Keep/select `Terminal / Bubble Tea` and press `Enter`.
4. Confirm `row_id = tui`.

Then close only the TUI with `Ctrl+C`.

PASS: daemon remains running.

## 5. Agent mutation while no renderer exists

With TUI closed and Web not started, Terminal B:

```bash
/tmp/air-r0-hw/bin/a2ui send \
  -server "$SERVER" \
  -session "$SESSION" \
  examples/air-r0-renderer-smoke-followup.ndjson
```

The follow-up continues the same operation sequence at 7..10. It changes:

- title to `AIR R0 hardware smoke — Web phase`;
- table order to `Local Web`, then `Terminal / Bubble Tea`;
- progress from 42% to 73%;
- commit frame to `air-r0-web-phase`.

It deliberately does **not** modify the input runtime value.

## 6. Direction B — local Web renderer

First prove that process attachment is not visibility.

Terminal C:

```bash
/tmp/air-r0-hw/bin/a2ui wait-event \
  -server "$SERVER" \
  -session "$SESSION" \
  -timeout 60s \
  -type committed \
  -raw
```

Terminal B:

```bash
/tmp/air-r0-hw/bin/a2ui web -socket "$SOCK"
```

Expected output resembles:

```text
AIR Web renderer: http://127.0.0.1:43123/
Press Ctrl+C to detach the Web renderer.
```

Do **not** open the URL immediately.

PASS: the `wait-event` process remains blocked. Starting the Web renderer process alone must not ACK human-visible publication.

Now open the printed `http://127.0.0.1:<port>/` URL in a browser on the same machine.

PASS: the waiting `committed` event now returns frame `air-r0-web-phase`.

Expected Web state:

- title is `AIR R0 hardware smoke — Web phase`;
- input value is still `from-tui`;
- table order is now `Local Web`, then `Terminal / Bubble Tea`;
- progress is approximately 73%.

The first two observations together are the central continuity proof:

```text
agent-owned semantic Document changed while detached
+
human-owned runtime input survived renderer replacement
```

### Web human-response test

Terminal C:

```bash
/tmp/air-r0-hw/bin/a2ui wait-event \
  -server "$SERVER" \
  -session "$SESSION" \
  -timeout 60s \
  -type submit \
  -raw
```

In the browser:

1. Replace the input value with `from-web`.
2. Press `Submit`.

PASS: Terminal C receives a `submit` event with `value = from-web` and `id = answer`.

## 7. Single interactive lease

While the Web renderer is still running, try in another terminal:

```bash
/tmp/air-r0-hw/bin/a2ui -socket "$SOCK"
```

PASS: the second interactive renderer is rejected with `ipc.client_busy` or the equivalent existing busy error. AIR must not silently create two interactive writers.

## 8. Reverse switch — Web back to TUI

Stop `a2ui web` with `Ctrl+C`. Do not stop the daemon.

Then:

```bash
/tmp/air-r0-hw/bin/a2ui -socket "$SOCK" -preset dashboard
```

PASS:

- TUI attaches successfully after Web releases the lease;
- title remains `AIR R0 hardware smoke — Web phase`;
- input value remains `from-web`;
- daemon PID did not change.

This closes the bidirectional renderer-lifetime proof:

```text
TUI -> detach -> Web -> detach -> TUI
```

with one daemon-owned semantic/runtime state.

## 9. Security boundary for this experiment

`a2ui web` is an experimental local renderer. The CLI accepts only numeric loopback listen addresses (`127.0.0.1` or `::1`). Do not reverse-proxy it, expose it to LAN/WAN, or treat this checkpoint as remote-Web security evidence.

Close the Web renderer after the test.

## 10. Acceptance result

Fill this after running on real hardware:

```text
AIR R0 RENDERER HARDWARE ACCEPTANCE

DATE:
MACHINE / OS:
EXACT HEAD:
BROWSER:
TERMINAL:

TUI initial render:                 PASS / FAIL
TUI committed-after-visible:       PASS / FAIL
TUI submit event:                  PASS / FAIL
TUI detach keeps daemon alive:     PASS / FAIL
Detached agent mutation:           PASS / FAIL
Web attach without visibility ACK: PASS / FAIL
Web visible publication ACK:       PASS / FAIL
Web sees retained from-tui input:  PASS / FAIL
Web sees detached Document update: PASS / FAIL
Web submit from-web:               PASS / FAIL
Single interactive lease:          PASS / FAIL
Web detach releases lease:         PASS / FAIL
TUI reattach sees from-web:        PASS / FAIL
Daemon PID unchanged end-to-end:   PASS / FAIL

FINAL VERDICT: PASS / FAIL
NOTES:
```

Until this section is filled from a real machine, the renderer experiment remains **CI GREEN / HARDWARE PENDING** rather than accepted.
