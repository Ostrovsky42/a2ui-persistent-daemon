# A2UI

> **Draft integration chapter.** The daemon and terminal client exist today. A packaged end-user install path and a no-JSON agent CLI are still prerequisites for an upstream-ready quickstart.

A2UI gives an agent a terminal interface without asking it to generate and execute a UI program first.

The agent describes a screen. `a2uid` keeps the semantic state. `a2ui` renders it in the terminal. Closing the window does not close the session.

## What it does

```text
agent
  ↓ six bounded UI mutations
 a2uid
  ↓ local Unix socket
 a2ui
  ↓
terminal UI
```

The terminal window is disposable. The daemon is not.

If you close `a2ui`, these stay in the daemon while it keeps running:

- the UI document;
- focus;
- input values;
- table selection;
- pending publication state.

Caret position, viewport scroll position, animation phase, and terminal size are local to the window and start fresh when a new client attaches.

## Install

**Not upstream-ready yet.** This repository does not currently provide the final Omarchy package or lazy-loaded launcher that an ordinary installed user should copy from the Manual.

For development, build the two existing binaries from the repository root:

```bash
go build -o ./bin/a2uid ./cmd/a2uid
go build -o ./bin/a2ui ./cmd/a2ui
```

Do not turn those development commands into an upstream Omarchy install instruction until packaging is implemented and tested outside the source checkout.

## Open a panel

Start the persistent daemon:

```bash
./bin/a2uid -session default
```

In another terminal, attach the renderer:

```bash
./bin/a2ui -preset dashboard
```

The default local socket is:

```text
$XDG_RUNTIME_DIR/a2ui/a2ui.sock
```

If `XDG_RUNTIME_DIR` is unavailable, A2UI uses a UID-scoped directory under `/tmp`.

Only one interactive renderer can hold the lease at a time.

## Make a choice

The current renderer supports keyboard-first interaction:

- `Tab` / `Shift+Tab` moves focus;
- arrow keys navigate a focused selectable table;
- `Home` / `End` jump within supported focused controls;
- typing edits a focused input;
- `Enter` submits an input or activates the selected table row;
- `PageUp` / `PageDown` scroll a focused scrollable viewport;
- `Ctrl+C` exits the terminal client.

A table activation becomes a semantic `select` event. Input submission becomes a semantic `submit` event. Those events belong to the daemon/runtime path, not to the terminal widget itself.

The agent interacts with the running session via the `a2ui` CLI:

```bash
# Agent publishes choice screen
a2ui send assets/examples/omarchy-choice.ndjson

# Agent waits for user decision
EVENT=$(a2ui wait-event --timeout 60s)
echo "Received: $EVENT"

# Agent updates the existing screen
cat <<JSON | a2ui send -
{"op":"text","id":"choice-title","text":"Deploying to staging..."}
{"op":"commit","frame":"in-progress"}
JSON
```

## Close and reopen

Exit the terminal client:

```text
Ctrl+C
```

Leave `a2uid` running, then attach again:

```bash
./bin/a2ui -preset dashboard
```

The new renderer receives the daemon's current atomic presentation snapshot. Semantic state survives; local caret and viewport offsets do not.

A second simultaneous client is rejected with:

```text
ipc.client_busy
```

## Stop the session

Stopping the terminal client is not the same as stopping the daemon.

To end the in-memory session, stop `a2uid` itself with `Ctrl+C` or `SIGTERM`.

A2UI does not currently promise disk persistence across daemon restart or system reboot.

## Security boundary

A2UI's public UI vocabulary is six mutations: `upsert`, `props`, `text`, `remove`, `focus`, and `commit`.

That vocabulary does not contain a shell, process-spawn, filesystem, or arbitrary-network primitive. Rendering an agent-described UI therefore does not require executing an agent-generated Bash or Go program.

That is a narrower claim than "the daemon is sandboxed." Registered host actions still run with host authority, the HTTP bridge has its own access-control requirements, and terminal-control sanitization for untrusted text is not yet proven.

See [Security model](../security-model.md) for the actual trust boundaries and [Security evidence](../security-evidence.md) for claim-by-claim status.

## Troubleshooting

| Symptom | What it means |
| --- | --- |
| `no Unix socket` | `a2uid` is not running at the path the client is using. |
| `ipc.client_busy` | Another interactive renderer owns the single-client lease. |
| `ipc.daemon_already_running` | A daemon already owns that socket path. Do not unlink its live socket. |
| client closes but state is gone | The daemon also stopped, or a different session/daemon was started. |
| commit appears pending | A renderer has not acknowledged the current publication generation yet. |
| HTTP bridge is exposed beyond loopback | Treat this as unsupported until an explicit authentication/access policy is added. |

## Status

The persistent daemon/client architecture is implemented and covered by Go tests. This Manual chapter remains a **draft** until all of the following are demonstrated on a clean Omarchy install:

1. packaged binaries runnable outside the repository;
2. a no-JSON agent-facing CLI round trip;
3. choice → user event → agent update → new rendered frame;
4. reconnect during that scenario;
5. installation and uninstall instructions;
6. terminal-control security review.
