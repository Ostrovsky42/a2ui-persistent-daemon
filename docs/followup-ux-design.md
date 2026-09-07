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
