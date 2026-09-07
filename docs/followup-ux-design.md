# P0.3 follow-up UX characterization

## Local acknowledgement

The Bubble Tea adapter already owns terminal-local mechanics in
`adapter/bubbletea.Model.interaction`; semantic authority remains in the
Engine. The first RED tests exercise `Model.Update` through the real
controller/Engine path and show that successful table Enter, input Enter, and
declared action dispatch emit their semantic event but leave the frame with no
acknowledgement or waiting presentation.

The intended minimal owner is adapter-local presentation state. It must retain
the acknowledged label/value until the next accepted renderer publication, and
must not modify Document, revision, publication generation, V1 messages, or
emit another event. This is sufficient for a later narrow implementation; no
protocol state machine is warranted.

## Viewer lifecycle today

Daemon viewer ownership is the single interactive `clientLease` in
`daemon/lease.go`. `Daemon.HasActiveClient()` is the existing `viewer=0`
observation. Accepted publication paths call `signalSnapshot`; the bounded
`updates` channel coalesces rapid signals. The daemon has no process-launch
dependency or terminal-launch interface.

`TestViewerlessPublicationSignalsCoalesce` records the current rapid case:
three viewerless signals leave one pending update and no viewer. It is a
characterization of the missing ownership boundary, not an auto-spawn feature.

## Terminal launcher ownership options

### A. Daemon owns a launcher interface

The daemon observes both publication and `viewer=0`, so it can serialize one
launch request. This makes the race easy to contain, but couples the semantic
daemon to host process policy and makes desktop replacement harder.

### B. Small local supervisor owns launch policy

A supervisor subscribes to daemon status/publication lifecycle and starts the
terminal client once while attach is pending. The daemon remains semantic-only;
desktop-specific policy can be replaced independently. It needs an explicit
local lifecycle contract and a reliable single-instance handoff.

### C. CLI/TUI bootstrap owns launch

The CLI can start a viewer when publishing, but it cannot reliably cover
publishes from real MCP/Codex or coordinate multiple publishers. It is the
least invasive but fails the intended agent-first workflow.

## Recommendation

Choose **B, a small local supervisor**. It preserves the daemon's current
semantic ownership and isolates future Omarchy/window policy. The supervisor
should be designed only after human approval; this experiment adds no launcher
code.

## Proposed supervisor contract (design only)

The following fills in the concrete lifecycle questions without selecting an
implementation. No daemon, protocol, or process-spawning code is introduced by
this document.

### Candidate A: daemon-coupled launcher callback

The daemon checks `publication_generation` after an accepted semantic
publication and `HasActiveClient() == false`, then invokes a local launcher
callback with `{session, server, socket}`. A daemon-local `launch_pending` bit
coalesces rapid generations and clears when the client lease attaches or the
launch process fails.

It has the simplest exact signal and race handling, but makes the semantic
daemon own desktop process policy. It is difficult for a future Omarchy adapter
to replace without touching daemon lifecycle code.

### Candidate B: one local supervisor per daemon (recommended)

`a2uid` and a separately started supervisor share a small **local lifecycle
descriptor**, not an A2UI V1 message. The descriptor contains immutable launch
arguments `{session, server URL, Unix socket, client preset}` and a local
read-only status source exposes `{publication_generation, has_client}`.

The supervisor's trigger is exactly:

```text
observed generation > last_observed_generation
and has_client == false
and launch_pending == false
```

`launch_pending` lives only in the supervisor process. It is set before spawn,
so generations 1, 2 and 3 create one launch request. It clears when either:

* the daemon reports a successful client lease (`has_client=true`); or
* the spawned terminal/client exits before a lease, in which case the failure
  is retained only as local diagnostic output and another *later generation*
  may make one new request.

When a user closes the viewer, `has_client` becomes false. A later generation
then satisfies the trigger and may launch one new terminal. The supervisor has
no ability to change Documents, events, revisions, or publication generations;
it observes them only. Startup is disabled by explicit `--disabled`, by a
headless/CI environment, and by SSH unless an explicit opt-in is supplied.
Desktop launching is behind a small `Launcher` interface, so a future Omarchy
implementation replaces only that interface and never A2UI/daemon semantics.

This option needs a new local status/descriptor contract, but keeps it outside
the frozen A2UI protocol and makes desktop policy independently replaceable.

### Candidate C: publisher/CLI bootstrap

Every publisher checks daemon status and starts `a2ui` when no client exists.
It can receive server/session/socket arguments directly from that publisher,
but independent MCP publishers cannot share `launch_pending`; rapid publishes
can open multiple terminals. It also misses non-CLI publication sources.

### Decision required before implementation

Candidate B is recommended. Before a `feat(launcher)`, approve the exact
local descriptor/status transport, supervisor lifetime and singleton strategy.
Those choices determine how `viewer=0` is observed and how the one-launch
invariant is actually enforced.
