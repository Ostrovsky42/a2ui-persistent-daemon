# A2UI UP — Local Session Supervisor Design

Date: 2026-09-07

Authoritative branch: `feature/20260907-a2ui-up-local-supervisor`

Inspected base: `96d54e40f6dae4e15aa7a932040d0b86cb1c12ef` (`feat(ux): show committed interaction while waiting`)

Document role: approved implementation design. Mutable execution status and verification evidence belong to the paired implementation plan and CI, not to a prose `Status:` claim in this design document.

## 1. Goal and non-goals

The normal local lifecycle becomes:

```text
a2ui up
  -> prepare/reuse one local A2UI environment
  -> return the caller shell
  -> keep daemon + supervisor alive independently
  -> on a new pending publication with no viewer, request one terminal viewer
  -> viewer acquires the existing interactive lease
  -> human interaction continues through the existing daemon/Engine path
```

The supervisor owns desktop viewer lifecycle only. It does not own Documents, mutations, events, revision, publication semantics, or agent transport semantics.

Non-goals for this checkpoint and its first Green phase:

- no Omarchy/window-manager integration;
- no systemd/autostart;
- no A2UI V2 or public protocol mutation;
- no focus/workspace/geometry policy;
- no viewer mute/cooldown/history/suppression policy;
- no UI acknowledgement redesign;
- no second semantic state store.

## 2. Existing ownership and evidence

### Semantic daemon

`daemon.Daemon` already owns the long-lived `Engine` and `Session`. Its own comment defines terminal clients as views that may disconnect without closing semantic state.

### Viewer lease

The authoritative viewer fact is `daemon.clientLease.active`. `Daemon.HasActiveClient()` is therefore the authoritative `viewer=0/1` observation. The lease rejects a second interactive client and is released on disconnect or explicit detach.

### Publication generation

`engine.Engine.publicationGeneration` is authoritative. `Engine.PublicationGeneration()` returns the current generation plus whether that semantic frame still needs visibility acknowledgement. `PublishGeneration()` only accepts the exact current token.

### Existing local observation surface

GET `/status` already returns:

```text
session
revision
nodes
has_client
generation
pending_publish
```

`agentclient.Client.Status()` already consumes this endpoint. The supervisor therefore does not need to consume semantic `/events`, attach to the interactive Unix IPC lease, or introduce another transport merely to observe publication and viewer state.

The existing Unix IPC cannot be used as a passive observer today: its `hello` path acquires the interactive lease. The daemon's internal `updates` channel is process-local and consumed by the active renderer path, so it is not an inter-process supervisor subscription.

### Existing startup serialization

`ipc.ListenUnix()` already serializes stale-socket probe/removal/bind with an advisory `flock` on `<socket>.lock`. It distinguishes an already-live daemon from stale socket state and must remain the authoritative socket bind race protection.

## 3. Observation options

### Option A — local lifecycle event stream

Pros:

- immediate wakeup;
- naturally expresses attach/detach/publication transitions;
- minimal steady-state reads.

Cons:

- no such passive stream exists today;
- reusing Unix IPC would currently steal the interactive lease;
- exposing `updates` across processes would require a new local IPC contract;
- a new stream introduces reconnection/cursor semantics for data that is already level-readable from `/status`.

Decision: reject for the first version.

### Option B — bounded status polling

Pros:

- reuses the existing `/status` transport and `agentclient.Status()` implementation;
- reads authoritative `generation`, `has_client`, and `pending_publish` without semantic ownership;
- monotonic generation means a polling interval cannot lose an accepted publication; rapid publications can be coalesced supervisor-locally;
- no A2UI V1 or semantic event change.

Cons:

- bounded detection latency;
- small steady-state local HTTP load;
- daemon loss is detected on the next poll rather than pushed immediately.

Decision: recommend this for the first version. Use a fixed 250 ms local polling interval and an independent 2 second request timeout; no busy loop. Poll cadence, individual request budget, and consecutive-failure grace are separate lifecycle parameters. The interval is an implementation constant, not a public semantic setting.

### Option C — hybrid wake + status read

Pros:

- status remains authoritative while a wake signal removes most polling.

Cons:

- the required cross-process wake signal does not exist today;
- adding a file/inotify side channel solely to avoid a small local status poll is worse than reusing `/status`;
- adding a dedicated lifecycle IPC signal now would expand the local contract without evidence that polling is a problem.

Decision: defer. It is a future optimization only if measurement justifies it.

## 4. Local status identity hardening

The trigger fields already exist, but process ownership needs one additional local-only identity guarantee: a supervisor must know that the HTTP status source belongs to the same daemon environment as the Unix socket it will launch the TUI against.

The first Green phase should extend the existing ergonomic `/status` response additively with immutable local runtime identity, not create a new transport and not modify `protocol.Version` or `ipc.Version`:

```text
instance_id   random per a2uid process lifetime
socket        canonical absolute Unix socket path
server        canonical local HTTP server URL/listen identity
```

`session` already exists. `instance_id` changes on daemon restart. `socket` and `server` are local lifecycle metadata only; they are not A2UI document or agent semantics.

This additive status hardening lets `a2ui up` adopt an already-running compatible new-version daemon, lets a supervisor detect daemon replacement, and prevents a coincidental process on the expected HTTP port from being treated as the socket owner. An older/unmanaged daemon that cannot prove the required local identity is classified as incompatible for managed `up`, not silently shadowed by a second daemon.

## 5. User lifecycle

`a2ui up` is a short-lived orchestrator, not the supervisor itself.

Under one startup transaction lock it:

1. Resolves the intended runtime descriptor from explicit flags/environment or the existing descriptor. Defaults remain session `default`, the existing resolved Unix socket, local HTTP server `http://127.0.0.1:8080`, and TUI preset `dashboard` unless existing explicit configuration says otherwise.
2. Resolves the installed A2UI binaries. The current `a2ui` executable should re-exec itself for supervisor mode rather than introduce a fourth shipped binary unless implementation evidence requires one. `a2uid` is resolved using the same installed-binary discipline already used by doctor/setup code.
3. Preflights Codex registration if Codex is installed. A conflicting registration fails before starting new background processes.
4. Classifies daemon state. Reuses a healthy compatible daemon or starts exactly one daemon and waits for readiness.
5. Reuses a healthy supervisor or starts exactly one supervisor and waits until its singleton lock/identity is established.
6. If Codex registration was absent and deterministic to create, reuses the existing `setup-codex` implementation without `--replace` semantics.
7. Prints short truthful status and returns the caller shell.

Representative output:

```text
A2UI ready
session: default
daemon: running (reused)
supervisor: running
Codex: configured
viewer: waiting
```

The wording is not contractual; truthfulness is.

## 6. Process ownership model

Three models were considered.

### A. Caller detaches children

A short-lived `a2ui up` starts daemon/supervisor and detaches them. This is necessary at the spawn boundary, but caller ownership must end after readiness. Shell job-control tricks such as `command & disown` are not product architecture.

### B. Supervisor owns daemon child

Rejected. It couples daemon lifetime to desktop policy and makes the required recovery case awkward: a dead supervisor must be restorable without restarting a healthy daemon.

### C. Independent daemon + supervisor processes

Recommended.

Both background processes are independent after `up` returns. On Unix, the starter uses explicit process detachment (new session/process group), redirects stdin away from the caller terminal, and routes stdout/stderr to deterministic private runtime logs. The supervisor does not restart the daemon and the daemon does not restart the supervisor.

Recovery ownership stays simple:

- supervisor dead, daemon healthy -> next `a2ui up` starts only supervisor;
- viewer launch fails or the launched viewer does not acquire the lease before its attach deadline -> supervisor records the failure and exits nonzero, releasing `supervisor.lock`; the pending publication remains daemon-owned, and the next `a2ui up` may start a fresh supervisor whose bootstrap performs one fresh attempt for that same still-pending generation;
- daemon dead/replaced -> bound supervisor detects status/instance loss, records a diagnostic, and exits after a short bounded failure grace; it never fabricates health or starts a new daemon;
- next `a2ui up` is the repair/reconciliation entrypoint.

## 7. Runtime state and descriptor

Use the directory containing the resolved default socket, preserving the existing XDG policy:

```text
$XDG_RUNTIME_DIR/a2ui/
```

with the existing `/tmp/a2ui-$UID/` fallback. Directory mode remains `0700`; local metadata/log/lock files are private (`0600` where applicable).

Local-only runtime files may include:

```text
environment.json       ephemeral lifecycle descriptor
up.lock                short-lived startup transaction lock
supervisor.lock        held for the supervisor lifetime
supervisor.pid         diagnostic only, never singleton authority
a2uid.pid              diagnostic only when up started the daemon
a2uid.log
supervisor.log
```

The descriptor contains only lifecycle configuration/identity, conceptually:

```text
local descriptor version
instance_id
socket
server
session
preset
viewer policy
```

It contains no Document, mutations, events, user content, or persistent semantic state. Writes are atomic temp-file + rename under the startup lock.

## 8. Singleton and concurrent `up`

Do not use `check PID -> start`.

`a2ui up` acquires `up.lock` with advisory `flock` for the complete reconciliation transaction. A second concurrent caller waits, then re-probes and reuses what the first caller established.

Daemon singleton remains protected by the existing `ipc.ListenUnix()` socket lock and live-socket logic; `up` does not duplicate stale-socket removal.

The supervisor acquires `supervisor.lock` non-blocking and holds it for its entire process lifetime. A second supervisor process exits as `already running`. PID files are diagnostic only and can be replaced after lock ownership proves stale state.

Expected concurrent invariant:

```text
two a2ui up callers
=> one socket owner
=> one daemon
=> one supervisor
=> both callers receive truthful final state
```

## 9. Daemon state classification

`up` must classify rather than blindly spawn:

### Not running

No live expected socket and no compatible local status owner. Start daemon.

### Running healthy/compatible

Unix socket is live, `/status` is reachable, required local status identity is present, `session/socket/server` match the environment, and the endpoint schema is compatible. Reuse it.

### Running incompatible

A live socket or status endpoint exists but identity/session/configuration is incompatible, or an older daemon cannot prove the required local identity. Stop with a useful diagnostic. Do not create a second daemon.

### Stale state

No live daemon, but stale descriptor/PID metadata exists. Under `up.lock`, replace local metadata. Socket staleness remains owned by `ipc.ListenUnix()`.

### Running but broken/unreachable

Examples: socket live but required status endpoint unreachable, expected HTTP address occupied by a non-A2UI service, status live but expected socket absent, or daemon identity changes during readiness. Report broken/incompatible state; do not spawn a second owner on top of it.

## 10. Supervisor state machine

The supervisor is bound to one immutable local environment identity for its lifetime.

Supervisor-local state is conceptually:

```text
last_observed_generation
launch_pending
launch_generation
failed_through_generation
attach_deadline
```

`launch_pending` is never daemon state and never written to A2UI protocol/document state.

### Bootstrap

On supervisor start, read authoritative status. Set `last_observed_generation` to the current generation. If the daemon already has `pending_publish=true`, `has_client=false`, generation > 0, and auto-spawn is enabled, perform one bootstrap launch attempt. This prevents a pending UI from being stranded merely because the previous supervisor died. An already-published generation is never replay-launched.

### Normal trigger

For subsequent status observations, the launch condition is:

```text
generation > last_observed_generation
AND pending_publish == true
AND has_client == false
AND launch_pending == false
AND generation > failed_through_generation
AND viewer auto-spawn policy allows desktop launch
```

`pending_publish` is an important guard: a generation that was already made visible and whose viewer subsequently closed must not reopen a terminal merely because a restarted observer sees the generation for the first time.

Advance `last_observed_generation` to the newest observed generation before requesting launch. Rapid generations therefore coalesce rather than queue terminal requests.

### Set pending

Set `launch_pending=true` before invoking the launcher. Store the triggering generation and start a bounded attach deadline.

### Clear pending on success

The preferred and authoritative success fact is `has_client=true`, meaning the existing daemon lease was acquired. Only then is the attempt considered attached and `launch_pending` cleared as success.

`has_client` intentionally proves lease acquisition, not desktop visibility. Human-visible attachment is guaranteed by the host-launcher boundary: automatic viewer launch must go through a supported terminal emulator rather than start the Bubble Tea process detached with `/dev/null` and a log file as its terminal.

### Failure before attachment

Two failure facts are supported:

1. the host launcher returns an immediate start error; or
2. the attach deadline expires and authoritative status still reports no client.

Before ending a failed pending attempt, use the latest observed status and set `failed_through_generation` to at least that current generation. This suppresses the triggering generation and any rapid generations that arrived while the launch was pending for the remainder of this supervisor lifetime. The same supervisor process never retries that semantic generation in its poll loop.

A failed launch or attach is also a lifecycle-fatal supervisor condition. Return the failure to the poller, log it, and exit nonzero so `supervisor.lock` is released. The daemon and pending publication remain alive. The next explicit `a2ui up` repair starts a fresh supervisor; bootstrap may then make one new launch attempt for the same still-pending generation. This makes recovery explicit without introducing a retry loop, a control API for resetting `failed_through_generation`, or supervisor-owned semantic state.

Retain a concise local diagnostic in the supervisor log. Do not emit a semantic A2UI event for host launch failure.

### Viewer closes

Lease release changes `has_client` to false. Detach alone does not launch. Only a later new pending publication satisfies the trigger and opens one viewer again.

### Daemon loss

A transient failed status read does not immediately fabricate daemon death. After a small bounded consecutive-failure grace, or immediately on a proven `instance_id` replacement/mismatch, the supervisor logs the loss and exits nonzero. It does not spawn a replacement daemon.

## 11. Terminal launcher boundary

The supervisor owns only a narrow host interface, conceptually:

```go
type Launcher interface {
    LaunchViewer(ctx context.Context, spec ViewerSpec) error
}
```

The exact Go spelling is implementation-derived. The first `ViewerSpec` should contain only what the existing TUI actually needs:

```text
A2UI executable path
Unix socket path
presentation preset
```

The TUI does not require agent session/server arguments to attach, so do not include them merely for symmetry. A title is optional and should not be added unless a backend actually requires it.

Launcher implementations must use argv-based process execution, not shell-composed command strings.

The first Linux backend discovery order is:

1. `xdg-terminal-exec` when present;
2. `gnome-terminal`;
3. `kitty`;
4. `alacritty`;
5. `konsole`.

Each emulator's argv differences remain isolated behind the launcher backend. `xdg-terminal-exec` and `gnome-terminal` receive the viewer after `--`; Kitty receives the viewer program directly; Alacritty and Konsole receive it after `-e`. The supervisor core never knows emulator names, focus APIs, workspaces, or geometry.

Current execution-environment characterization found none of those terminal binaries. It also reports `CI=true` while `DISPLAY=:0`, which is useful evidence that CI detection must override a superficial display variable. Real desktop-window acceptance therefore belongs to the later manual host phase; CI verifies discovery order and argv construction with hermetic fake terminal executables rather than opening real windows.

## 12. Headless, SSH, CI, and explicit policy

Default viewer policy is `auto`.

Auto-spawn is disabled when any of these minimal conditions is true:

```text
CI is non-empty
SSH_CONNECTION or SSH_TTY is non-empty
both DISPLAY and WAYLAND_DISPLAY are empty
```

Do not build a desktop-environment detector.

Expose one explicit viewer policy at `a2ui up`, conceptually `auto | never | always`:

- `auto`: apply the minimal detector above;
- `never`: supervisor runs but never auto-launches a terminal;
- `always`: explicit user/dev opt-in that bypasses headless/SSH/CI suppression but still requires an available launcher.

Tests inject the detector and fake launcher rather than opening real terminals.

## 13. Codex integration boundary

`a2ui up` is not a Codex daemon. Supervisor and daemon contain no Codex logic.

Reuse existing `readCodexMCPConfig`, validation, `setupCodex`, and doctor behavior rather than duplicate configuration mutation.

Policy:

1. If Codex is installed and the existing `a2ui` MCP registration exactly matches the resolved `a2ui-mcp`, server, and session: no-op.
2. If Codex is installed and registration is missing: call the existing deterministic setup path without replacement semantics.
3. If a conflicting registration exists: stop with the existing useful replace/inspection instruction. Never silently use `--replace` and never mutate unrelated Codex configuration.
4. If Codex is not installed: core A2UI runtime may still become ready and reports `Codex: unavailable/not installed`; daemon + supervisor remain agent-neutral.
5. If Codex is present but `a2ui-mcp` is missing, the current Codex integration cannot be claimed ready; report it as an actionable setup failure.

Conflicting registration is preflighted before starting new background processes where possible, so `up` does not create a new environment and then immediately fail on a known integration conflict.

## 14. First RED matrix for the approved implementation pass

Production implementation must begin with fake/injected process and launcher dependencies. Real terminal windows are forbidden in unit/CI tests.

### RED 1 — idempotent up

First `up` starts one environment; second reuses it. Assert one daemon owner and one supervisor owner.

### RED 2 — viewerless publication

No viewer + new pending generation -> exactly one `LaunchViewer` call.

### RED 3 — viewer already attached

Viewer attached + new generation -> zero launcher calls.

### RED 4 — rapid publications

No viewer; generations 1/2/3 before attachment -> one launcher call maximum while pending.

### RED 5 — viewer attaches

Pending launch + authoritative `has_client=true` -> pending clears as success.

### RED 6 — viewer closes, later publication

Attach -> detach with same generation -> no launch. Later new pending generation -> exactly one new launch.

### RED 7 — launch failure and repair

Immediate launcher error or attach timeout -> local failure diagnostic retained, failed-through watermark advanced, and the supervisor returns an error so its process exits rather than entering a stable failed state. Continuing the same controller lifetime cannot relaunch the failed generation. A fresh supervisor may bootstrap the same still-pending generation exactly once.

### RED 8 — concurrent up

Two startup attempts race -> one daemon, one supervisor, one socket owner, both callers obtain truthful terminal status.

Additional hardening tests should cover daemon instance replacement, stale runtime metadata, headless suppression, independent status-request timeout, terminal discovery/argv contracts, and conflicting Codex registration without broadening the checkpoint.

The existing `TestViewerlessPublicationSignalsCoalesce` remains a useful characterization of the daemon boundary but is not a substitute for these supervisor REDs.

## 15. Manual acceptance after Green implementation

Run on a real Linux graphical desktop with an actually installed supported terminal backend.

1. Start from no managed environment. `a2ui up` returns the shell and reports started daemon/supervisor.
2. Run `a2ui up` again. Verify process counts and socket owner do not increase.
3. Publish from the existing Codex/MCP workflow with no viewer. Exactly one terminal opens and attaches.
4. Publish several generations rapidly before attachment. Still one terminal maximum.
5. With a viewer attached, publish again. Existing UI updates; no terminal is opened/raised/focused.
6. Close viewer. Verify no immediate reopen. Publish a later semantic UI; one terminal opens again.
7. Force terminal-launch failure or attach timeout. Verify the supervisor logs the failure, exits, and releases its singleton lock while the daemon and publication remain alive. Run `a2ui up`; verify it reuses the daemon, starts a fresh supervisor, and bootstrap makes one new attempt for that same pending generation.
8. Kill supervisor only. `a2ui up` restores supervisor without restarting daemon or losing semantic state.
9. Kill daemon. Verify supervisor reports/logs daemon loss and exits rather than claiming health. `a2ui up` reconciles a new environment.
10. Exercise `--viewer-policy=never`; verify no terminal spawn.
11. Exercise SSH/headless/CI-like environment; verify auto-spawn suppression.
12. Inspect runtime directory/file permissions and deterministic logs.

## 16. Preservation gates

The implementation must keep these exact boundaries:

- `protocol.Version` remains 1;
- A2UI node types, mutations, event fields, Document semantics, and publication semantics are unchanged;
- P0.3 acknowledgement/waiting UX remains unchanged except for lifecycle integration needed to launch the existing TUI;
- daemon contains no terminal-emulator, focus, workspace, geometry, Omarchy, or window-manager policy;
- supervisor does not consume or own semantic events;
- `launch_pending` and `failed_through_generation` remain supervisor-local;
- a failed automatic viewer attachment cannot leave a healthy lock-owning supervisor with an unrecoverable pending publication;
- systemd/autostart remains outside scope.

## 17. Recommendation

Use Option B observation: one independent local supervisor using bounded reads of the existing `/status` path, with a minimal additive local runtime-identity hardening of that same status response. Keep daemon and supervisor as independent processes reconciled by `a2ui up`. Use advisory locks, not PID check/start races. Treat the viewer lease as the authoritative attach fact while requiring automatic launches to cross a real terminal-emulator boundary. Suppress repeated launch attempts for a failed generation within one supervisor lifetime, then exit on launch/attach failure so explicit `a2ui up` repair can bootstrap the same still-pending generation in a fresh lifetime. Keep terminal-specific logic behind a Linux launcher boundary.

This is the narrowest design that satisfies idempotent startup, crash recovery, one-launch coalescing, agent neutrality, and the frozen A2UI V1 boundary without inventing another lifecycle transport.
