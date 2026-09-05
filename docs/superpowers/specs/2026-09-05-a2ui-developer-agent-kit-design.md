# A2UI Developer Harness and Agent Kit Design

## Goal

Make the persistent-daemon workflow reproducible from the Go module root for
humans, CI, Codex, and Claude without duplicating socket lifecycle or MCP
protocol knowledge across tools.

## Scope

This is developer tooling around the existing Epic 1 daemon. It does not
change the A2UI wire protocol, daemon ownership model, Bubble Tea renderer,
or production deployment model.

The harness provides stable commands for starting a local daemon, attaching a
client, exercising the MCP HTTP bridge, exercising the Unix-socket handshake,
and running focused verification gates. It must run from the module root,
where `go.mod` resides.

## Architecture

`Makefile` is the sole public command surface. Each target delegates to one
shell entrypoint under `scripts/dev/`; targets do not embed JSON payloads,
socket paths, or process-cleanup logic.

`scripts/dev/a2ui` owns all local runtime setup. It creates an isolated
temporary directory with mode `0700`, derives the socket path below it,
starts `a2uid` with an explicit session, waits for readiness, and cleans up
only resources it created. For archive checkouts it invokes Go with
`-buildvcs=false`; no command requires a surrounding Git checkout.

The HTTP smoke case uses the existing stateless MCP bridge: it sends
`a2ui/hello`, asserts `a2ui/hello_ack`, sends one `a2ui/operation`, and
asserts `204 No Content`. The IPC smoke case sends a versioned `hello` record
over the Unix socket and asserts that `hello_ack` and `snapshot` are received.
It is intentionally not a renderer test.

## Make Targets

| Target | Contract |
| --- | --- |
| `make daemon` | Start `a2uid` in the foreground with an isolated socket directory and print the resolved endpoints. |
| `make client` | Attach `cmd/a2ui` to a supplied `SOCK` path; reject an empty or non-socket path before launching. |
| `make smoke-http` | Start a temporary daemon, run the MCP hello and operation assertions, then remove its process and temporary files. |
| `make smoke-ipc` | Start a temporary daemon, run the raw IPC hello/snapshot assertion, then remove its process and temporary files. |
| `make test` | Run `go test ./... -count=1`. |
| `make test-race` | Run `go test -race ./... -count=1`. |
| `make test-reattach` | Run the persistent daemon reattach E2E test. |
| `make test-startup-stress` | Run the concurrent Unix listener test with `-count=500`. |

`PORT`, `SMOKE_PORT`, `SESSION`, `SOCK`, and `PRESET` may be overridden through
Make variables. Foreground daemon defaults are `PORT=8080`, `SESSION=smoke`,
and `PRESET=dashboard`; isolated smoke targets default to `SMOKE_PORT=18080`
so they do not collide with the foreground development daemon.
The foreground daemon target derives `SOCK` if omitted and displays it for a
second terminal. Temporary smoke targets must never use a socket directly in
`/tmp`, because `a2uid` correctly secures the socket parent directory.

## Agent Kits

The common contract lives in `docs/agent-kit.md` and points only to Make
targets. It includes a minimal operating sequence, expected success signals,
and recovery steps for wrong module root, a missing socket, a busy client,
and a stale daemon.

`.codex/skills/a2ui-daemon/SKILL.md` and `.claude/skills/a2ui-daemon/SKILL.md`
are thin provider-specific entrypoints. They must not contain MCP JSON,
shell cleanup code, or duplicated protocol rules; both link to the common
contract and use the same Make targets. Each states that no full-release
claim is allowed while `make test` or `make test-race` fails.

## Error Handling and Safety

- Validate dependencies (`go`, `curl`, `jq`, and `socat`) with an actionable
  message before a target that needs them.
- Record daemon output in a temporary log and print it when readiness or a
  smoke assertion fails.
- Trap `INT`, `TERM`, and normal exit to stop only the daemon process spawned
  by that invocation.
- Do not remove a caller-provided `SOCK` path or its parent directory.
- Do not start a second daemon on an occupied path; surface the daemon's
  `ipc.daemon_already_running` diagnostic.

## Verification

Tooling changes require `make smoke-http`, `make smoke-ipc`, focused reattach
and startup-stress tests, then `make test`, `make test-race`, and `go vet ./...`.
The two existing Bubble Tea renderer failures are a separate release blocker;
their observed result must be reported rather than hidden or weakened by this
harness.
