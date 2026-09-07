# A2UI UP — Local Supervisor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement `a2ui up` as an idempotent local environment reconciler plus an independent viewer-lifecycle supervisor that opens exactly one existing A2UI TUI terminal for a new pending publication only when no viewer is attached.

**Architecture:** Preserve the frozen A2UI V1 semantic path. The daemon remains the sole semantic owner; `/status` gains only local runtime identity. A new supervisor component polls that existing status surface every 250 ms, owns `launch_pending` locally, and calls a narrow argv-based Linux launcher. `a2ui up` is a short-lived reconciler that serializes startup with `flock`, reuses or starts independent daemon/supervisor processes, reuses existing Codex setup helpers, writes only ephemeral runtime metadata, and returns the caller shell after readiness.

**Tech Stack:** Go 1.24+, `net/http`, Unix domain sockets, `syscall.Flock`, `os/exec`, `syscall.SysProcAttr{Setsid:true}`, existing `agentclient`, existing `ipc.ListenUnix`, existing `setupCodex`/doctor helpers, GitHub Actions CI for authoritative verification in this execution environment.

**Spec:** `docs/superpowers/specs/2026-09-07-a2ui-up-local-supervisor-design.md` at approved commit `7b8537e70eaf572e7e9b2703e3a36f5c9bc62c6e`.

## Global Constraints

- `protocol.Version` remains `1`.
- `ipc.Version` remains unchanged.
- Document, revision, publication, and event semantics remain unchanged.
- No new `NodeType`, mutation, or semantic event field.
- Daemon remains semantic owner; supervisor remains viewer-lifecycle owner.
- Preserve P0.3 acknowledgement/waiting UX (`✓ Selected`, `✓ Submitted`, `✓ <Action> requested`, `⠋ Waiting for agent…`).
- No Omarchy, systemd, login autostart, focus stealing, workspace policy, browser UI, multiple viewers, or persistent semantic supervisor state.
- Every production behavior follows RED → proved RED → minimal GREEN → focused verification → commit.

---

## File Map

### G1 — local status identity + supervisor core

- Modify: `daemon/daemon.go` — daemon-local runtime identity storage; instance ID generation remains outside Engine.
- Modify: `daemon/mcp.go` — additive `/status` JSON fields only.
- Modify: `cmd/a2uid/main.go` — bind actual socket/server identity into the daemon instance.
- Modify: `agentclient/client.go` — consume additive status identity fields.
- Create: `daemon/status_identity_test.go` — RED/GREEN coverage for `instance_id`, socket, server, process-lifetime immutability and restart identity change.
- Create: `agentclient/status_identity_test.go` — client decode contract for additive status fields.
- Create: `supervisor/supervisor.go` — local state machine, status/launcher abstractions, identity binding, coalescing/failure rules.
- Create: `supervisor/supervisor_test.go` — S1–S8 and daemon replacement tests with fakes only.
- Create: `supervisor/policy.go` and `supervisor/policy_test.go` — `auto|never|always` and CI/SSH/headless suppression.

### G2 — `a2ui up` lifecycle

- Create: `localenv/runtime.go` — runtime paths and environment descriptor.
- Create: `localenv/runtime_test.go` — descriptor permission/atomic-write/stale-metadata behavior.
- Create: `localenv/lock_unix.go` and `localenv/lock_unix_test.go` — blocking startup lock and non-blocking lifetime lock via advisory `flock`.
- Create: `localenv/reconcile.go` and `localenv/reconcile_test.go` — daemon state classification and idempotent/concurrent reconciliation with injected process/status/socket dependencies.
- Create: `cmd/a2ui/up_cli.go` and `cmd/a2ui/up_cli_test.go` — `a2ui up`, concise output, independent process detachment, Codex preflight/reuse, supervisor subcommand wiring.
- Modify: `cmd/a2ui/main.go` — dispatch `up` and internal supervisor command.
- Modify: `cmd/a2ui/connectability.go` — add an idempotent Codex ensure helper that reuses existing read/validate/setup functions without silent replacement.
- Modify: `cmd/a2ui/connectability_cli_test.go` or add focused `cmd/a2ui/up_codex_test.go` — correct/missing/conflicting/absent Codex cases.

### G3 — Linux launcher + manual acceptance

- Create: `supervisor/launcher_linux.go` and `supervisor/launcher_linux_test.go` — backend discovery and argv construction for `xdg-terminal-exec`, `gnome-terminal`, `kitty`, `alacritty`, `konsole`.
- Create: `docs/acceptance/A2UI_UP_LOCAL_SUPERVISOR.md` — exact manual desktop acceptance commands and expected observations.
- Modify: `README.md` / `docs/manual/a2ui.md` only if needed to expose `a2ui up` without broad documentation refactors.

---

# G1 — Local Status Identity + Supervisor Core

### Task 1: Add local daemon status identity

**Files:**
- Create: `daemon/status_identity_test.go`
- Create: `agentclient/status_identity_test.go`
- Modify: `daemon/daemon.go`
- Modify: `daemon/mcp.go`
- Modify: `cmd/a2uid/main.go`
- Modify: `agentclient/client.go`

**Interfaces:**
- Produces: additive `agentclient.Status.InstanceID`, `.Socket`, `.Server` strings.
- Produces: daemon runtime identity configured by `a2uid`, with random instance ID created once per daemon instance/process lifetime.
- Preserves: existing `/status` fields and all public A2UI/IPC versions.

- [ ] **Step 1: Write the failing daemon status test**

Create an HTTP-level test that constructs two daemon instances with the same session/socket/server configuration and asserts:

```go
status1 := decodeStatus(t, daemon1)
status1Again := decodeStatus(t, daemon1)
status2 := decodeStatus(t, daemon2)

if status1.InstanceID == "" { t.Fatal("missing instance_id") }
if status1.InstanceID != status1Again.InstanceID { t.Fatal("instance_id changed within daemon lifetime") }
if status1.InstanceID == status2.InstanceID { t.Fatal("restart identity did not change") }
if status1.Socket != "/tmp/a2ui-test/a2ui.sock" { t.Fatalf("socket=%q", status1.Socket) }
if status1.Server != "http://127.0.0.1:18080" { t.Fatalf("server=%q", status1.Server) }
```

The test must use the existing `/status` handler and fail on missing fields, not on compilation scaffolding.

- [ ] **Step 2: Prove RED**

Run authoritative CI/focused test:

```bash
go test ./daemon/... ./agentclient/... ./cmd/a2uid/... -count=1
```

Expected: FAIL because `/status` does not yet expose the approved local identity.

- [ ] **Step 3: Minimal GREEN**

Implement daemon-local runtime metadata. Generate the instance ID with `crypto/rand` (for example 16 random bytes hex-encoded) once during daemon construction. Store socket/server strings on `daemon.Daemon`, not `engine.Engine`. Add a narrow constructor/configuration path used by `cmd/a2uid` so the handler reports the actual resolved Unix socket and configured local HTTP identity. Extend `serveStatus` additively:

```go
"instance_id": d.InstanceID(),
"socket":      d.RuntimeSocket(),
"server":      d.RuntimeServer(),
```

Extend `agentclient.Status` with matching JSON fields. Do not touch protocol envelopes or IPC messages.

- [ ] **Step 4: Verify GREEN**

```bash
go test ./daemon/... ./agentclient/... ./cmd/a2uid/... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add daemon agentclient cmd/a2uid
git commit -m "feat(status): expose local daemon identity"
```

### Task 2: Characterize supervisor state machine (S1–S8 + replacement)

**Files:**
- Create: `supervisor/supervisor_test.go`

**Interfaces:**
- Test-facing desired interfaces:

```go
type StatusSource interface {
    Status(context.Context) (agentclient.Status, error)
}

type Launcher interface {
    LaunchViewer(context.Context, ViewerSpec) error
}

type ViewerSpec struct {
    Executable string
    Socket     string
    Preset     string
}
```

- The exact internal controller API may differ, but tests must observe real state-machine behavior, not mock call implementation details alone.

- [ ] **Step 1: Write RED S1–S8 and daemon replacement tests**

Cover:

```text
S1 generation N→N+1 + pending + viewer=0 => one launch
S2 generation increase + viewer=1 => zero launch
S3 N+1/N+2/N+3 before attach => one launch
S4 authoritative has_client=true => pending clears
S5 true→false same generation => no launch; later new pending generation => one launch
S6 immediate launcher error => failed-through generation suppresses same generation; later generation retries once
S7 attach deadline expires => failure recorded; same generation suppressed
S8 bootstrap pending publication => one launch; bootstrap non-pending => zero launch
identity instance/socket/server/session mismatch => controller/runner terminates with replacement/incompatible diagnostic
```

Use fake clock or explicit `now` injection so attach timeout tests do not sleep.

- [ ] **Step 2: Prove RED**

```bash
go test ./supervisor -count=1
```

Expected: FAIL because supervisor production component does not exist.

- [ ] **Step 3: Commit RED characterization if repository discipline permits a red-only commit**

```bash
git add supervisor/supervisor_test.go
git commit -m "test(supervisor): characterize managed lifecycle"
```

### Task 3: Implement minimal supervisor core and policy

**Files:**
- Create: `supervisor/supervisor.go`
- Create: `supervisor/policy.go`
- Create: `supervisor/policy_test.go`
- Modify: `supervisor/supervisor_test.go` only to fix test defects, never to weaken approved expectations.

**Interfaces:**
- Consumes: `agentclient.Status` identity/publication/viewer fields.
- Produces: `Controller.Observe(ctx, status, now)` or equivalent deterministic observation method plus `Run(ctx, source)` fixed-interval loop.
- Produces: viewer policy parser/evaluator for `auto|never|always`.

- [ ] **Step 1: Implement the smallest state machine that satisfies the REDs**

State remains controller-local:

```go
lastObservedGeneration uint64
launchPending           bool
launchGeneration        uint64
failedThroughGeneration uint64
attachDeadline          time.Time
initialized             bool
```

The trigger must remain exactly the approved conjunction. Set pending before launcher invocation. `has_client=true` is the only attach-success authority. On failure, suppress through the latest observed generation. Bootstrap may launch once only when current status is pending, viewerless, generation > 0, and policy allows launch.

- [ ] **Step 2: Implement runner polling**

Use a fixed production interval of `250*time.Millisecond`. A status read timeout/error is local lifecycle failure only. Allow a small fixed consecutive-read grace; proven instance identity replacement fails immediately. The runner returns nonzero/error and never restarts the daemon.

- [ ] **Step 3: Implement policy tests RED then GREEN**

Assertions:

```text
auto + CI => suppressed
auto + SSH_CONNECTION => suppressed
auto + SSH_TTY => suppressed
auto + no DISPLAY/WAYLAND_DISPLAY => suppressed
auto + graphical local env => allowed
never => suppressed
always => allowed even in CI/SSH/headless
```

- [ ] **Step 4: Verify focused GREEN**

```bash
go test ./supervisor ./daemon/... ./agentclient/... -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add supervisor
git commit -m "feat(supervisor): manage viewer launch lifecycle"
```

---

# G2 — `a2ui up` Lifecycle

### Task 4: Characterize runtime metadata and advisory locks

**Files:**
- Create: `localenv/runtime_test.go`
- Create: `localenv/lock_unix_test.go`

**Interfaces:**
- Desired descriptor:

```go
type Descriptor struct {
    Version    int    `json:"version"`
    InstanceID string `json:"instance_id"`
    Socket     string `json:"socket"`
    Server     string `json:"server"`
    Session    string `json:"session"`
    Preset     string `json:"preset"`
    Viewer     string `json:"viewer"`
}
```

- Desired runtime paths are derived from the directory containing `ipc.ResolveSocketPath(...)`.

- [ ] **Step 1: Write failing runtime tests**

Assert directory creation is `0700`, descriptor/private files are `0600`, descriptor replacement is atomic from the reader perspective, stale PID files have no authority, blocking `up.lock` serializes callers, and non-blocking `supervisor.lock` rejects a second lifetime owner.

- [ ] **Step 2: Prove RED**

```bash
go test ./localenv -count=1
```

Expected: FAIL because package is not implemented.

### Task 5: Implement runtime files and locks

**Files:**
- Create: `localenv/runtime.go`
- Create: `localenv/lock_unix.go`

**Interfaces:**
- Produces: runtime path resolver; atomic descriptor read/write; advisory blocking/non-blocking lock handles.

- [ ] **Step 1: Minimal GREEN**

Use `os.MkdirAll(..., 0700)` + `os.Chmod`, private files `0600`, temp file in same directory, encode, `Sync`, `Close`, `Rename`, and directory sync where supported. Use `syscall.Flock` for `up.lock` and `supervisor.lock`; keep the lock file open for ownership lifetime.

- [ ] **Step 2: Verify**

```bash
go test ./localenv -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add localenv
git commit -m "feat(localenv): add runtime descriptor and lifecycle locks"
```

### Task 6: Characterize `a2ui up` reconciliation (U1–U6)

**Files:**
- Create: `localenv/reconcile_test.go`
- Create: `cmd/a2ui/up_cli_test.go`

**Interfaces:**
- Inject process starter, status probe, Unix socket probe, executable resolver, and Codex ensure function so tests never spawn real background processes.

- [ ] **Step 1: Write RED U1–U6**

Cover first startup, idempotent second up, concurrent up, daemon healthy/supervisor missing, incompatible daemon refusal, stale metadata recovery. Assert daemon/supervisor spawn counts and truthful result fields.

- [ ] **Step 2: Prove RED**

```bash
go test ./localenv ./cmd/a2ui -count=1
```

Expected: FAIL because reconciliation/CLI is not implemented.

### Task 7: Implement `a2ui up`, independent process model, and Codex reuse

**Files:**
- Create: `localenv/reconcile.go`
- Create: `cmd/a2ui/up_cli.go`
- Modify: `cmd/a2ui/main.go`
- Modify: `cmd/a2ui/connectability.go`
- Add/modify focused Codex tests under `cmd/a2ui`.

**Interfaces:**
- `a2ui up` options: server, session, socket, preset, viewer policy.
- Internal supervisor subcommand re-execs the current `a2ui` executable with resolved server/socket/session/preset/viewer identity and holds `supervisor.lock` for its lifetime.
- Background daemon is `a2uid -socket <path> -server <host:port> -session <session>`.

- [ ] **Step 1: Implement daemon classification**

Classify:

```text
not running => start
healthy compatible => reuse
live socket or reachable HTTP with identity/session/socket/server mismatch => fail
status reachable but socket absent => fail
socket live but status unreachable => fail
neither live => may start
```

Never duplicate `ipc.ListenUnix` stale-socket removal.

- [ ] **Step 2: Implement process detachment**

Use argv-based `exec.Cmd`, `SysProcAttr.Setsid=true`, stdin from `/dev/null`, stdout/stderr to deterministic `0600` runtime logs. Start daemon and supervisor independently; neither owns/restarts the other. `a2ui up` waits only for bounded readiness and then exits.

- [ ] **Step 3: Implement idempotent Codex ensure**

Reuse `readCodexMCPConfig`, `validateCodexMCPConfig`, and `setupCodex`:

```text
correct existing => no-op configured
missing registration => setupCodex without replace
conflict => actionable error, never replace
codex absent => core ready + unavailable state
a2ui-mcp absent while codex exists => actionable setup failure
```

- [ ] **Step 4: Implement concise truthful output**

Output fields only after probes confirm them, for example:

```text
A2UI ready
session: default
daemon: running (reused)
supervisor: running
Codex: configured
viewer: waiting
```

- [ ] **Step 5: Verify focused GREEN**

```bash
go test ./localenv ./cmd/a2ui ./supervisor -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add localenv cmd/a2ui
git commit -m "feat(cli): add a2ui up reconciliation"
```

---

# G3 — Linux Terminal Launcher + Acceptance

### Task 8: Verify backend argv contracts, then write launcher REDs

**Files:**
- Create: `supervisor/launcher_linux_test.go`

**Interfaces:**
- Discovery order: `xdg-terminal-exec`, `gnome-terminal`, `kitty`, `alacritty`, `konsole`.
- Viewer argv payload: installed/current `a2ui`, `-socket`, resolved socket, `-preset`, resolved preset.

- [ ] **Step 1: Verify real documented argv syntax before coding**

Record the source-backed syntax in the test comments or acceptance doc. Do not infer shell syntax.

- [ ] **Step 2: Write failing tests**

Inject executable lookup and process start. Assert discovery order and exact argv, including socket/executable values containing spaces, quotes, `$`, `;`, and other shell metacharacters as literal argv values. No test invokes a real terminal.

- [ ] **Step 3: Prove RED**

```bash
go test ./supervisor -run 'Launcher|Policy' -count=1
```

Expected: FAIL because Linux launcher is absent.

### Task 9: Implement Linux launcher and headless integration

**Files:**
- Create: `supervisor/launcher_linux.go`
- Modify: supervisor command wiring in `cmd/a2ui/up_cli.go` only as required to instantiate the launcher.

**Interfaces:**
- Produces: `LinuxLauncher` satisfying supervisor `Launcher`.

- [ ] **Step 1: Minimal GREEN**

Use `exec.LookPath` and `exec.CommandContext`/`Start` with argument slices. No `sh -c`, no generated command strings. Preserve backend-specific argv only inside launcher code.

- [ ] **Step 2: Verify focused GREEN**

```bash
go test ./supervisor ./cmd/a2ui -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add supervisor cmd/a2ui
git commit -m "feat(launcher): add Linux terminal viewer backend"
```

### Task 10: Preserve semantic/UI invariants and document manual gate

**Files:**
- Create: `docs/acceptance/A2UI_UP_LOCAL_SUPERVISOR.md`
- Modify: `README.md` and/or `docs/manual/a2ui.md` only if a narrow command reference is needed.

- [ ] **Step 1: Add exact manual commands**

Document build/install, teardown of any previous managed environment, `a2ui up` twice, process/socket checks, real Codex publication, one-terminal observation, P0.3 acknowledgement/waiting observation, same-terminal second publication, manual close/no same-generation reopen, and new-generation reopen.

- [ ] **Step 2: Commit docs**

```bash
git add docs README.md
git commit -m "docs: add a2ui up manual acceptance"
```

### Task 11: Full automated preservation matrix

**Files:** none unless a real failing test identifies a defect.

- [ ] **Step 1: Format/build/tests/vet**

```bash
gofmt -w <all changed .go files>
make build
go test ./...
go test -race ./...
go vet ./...
```

- [ ] **Step 2: Fuzz smoke**

Use the repository's existing exact fuzz smoke commands/workflow for wire, document reducer, and IPC fuzz targets. Do not invent replacement fuzz targets.

- [ ] **Step 3: Review semantic diff**

Confirm no changes to `protocol.Version`, `ipc.Version`, NodeTypes, mutation/event schemas, Document/revision/publication semantics, or P0.3 acknowledgement strings/behavior.

- [ ] **Step 4: Verification commit only if required**

Do not create a no-op commit. Any defect found here gets its own RED→GREEN repair commit.

- [ ] **Step 5: Stop at manual gate**

Return the required `A2UI UP — AUTOMATED GREEN` report with `MANUAL DESKTOP ACCEPTANCE: OPEN` and exact commands. Do not claim final desktop success until the user observes the real terminal behavior.
