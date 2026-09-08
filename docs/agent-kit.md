# A2UI agent operating contract

Use this document from the Go module root: the directory containing `go.mod`
and `Makefile`.

For first-time Codex onboarding, prefer [codex-mcp.md](codex-mcp.md). The
supported high-level path is:

```bash
make install
a2ui setup-codex
# start daemon + terminal client
a2ui doctor
```

Use this document for lower-level daemon and verification operations.

## Fast verification

Before reporting daemon transport success, run both independent smoke checks:

```bash
make smoke-http
make smoke-ipc
```

Each target starts a disposable daemon on `SMOKE_PORT=18080`, prints `PASS`,
and removes the daemon, socket directory, binary, and log it created. If that
port is occupied, choose another value:

```bash
make smoke-http SMOKE_PORT=18081
make smoke-ipc SMOKE_PORT=18081
```

The HTTP smoke proves the A2UI envelope `hello` / `hello_ack` path and one
operation. The IPC smoke proves the Unix-socket `hello_ack` and atomic initial
snapshot. Neither check renders a Bubble Tea terminal or proves standard MCP
tool discovery.

## Interactive daemon and client

In terminal one, start a daemon. The command creates an isolated socket
directory and prints its socket path.

```bash
make daemon PORT=8080 SESSION=smoke
```

Copy the printed path into terminal two and attach the terminal client:

```bash
make client SOCK=/tmp/a2ui-dev.example/a2ui.sock PRESET=dashboard
```

Only one interactive client may hold the lease. Close the current client
before attaching another. Do not delete the socket by hand; stop the daemon
that owns it.

## Real agent connection

`a2ui-mcp` is the standard MCP stdio facade for real harnesses. It exposes
exactly:

```text
a2ui_publish
a2ui_wait_event
a2ui_status
```

Build/install it with the other binaries:

```bash
make build
make install
```

Register it in Codex through the supported CLI path:

```bash
a2ui setup-codex --server http://127.0.0.1:8080 --session smoke
```

Then, after daemon/client startup, verify the full local preflight:

```bash
a2ui doctor --server http://127.0.0.1:8080 --session smoke
```

Use `a2ui doctor --json` for machine-readable diagnostics. The doctor validates
the Codex registration, daemon/session binding, and terminal attachment; it
reports optional smoke dependencies separately as WARN.

The deterministic shell demo remains useful for regression testing, but it does
not replace the real Codex + real human acceptance gate in
[`acceptance/CODEX_HUMAN_P0_1.md`](acceptance/CODEX_HUMAN_P0_1.md).

## Focused verification

```bash
make test-reattach
make test-startup-stress
go vet ./...
```

`test-reattach` proves that daemon-owned semantic state and a pending
publication barrier survive a client reconnect. `test-startup-stress` runs
500 concurrent listener starts to cover the socket startup lock.

The staged Omarchy protocol fixture has its own conformance replay:

```bash
go test ./conformance -run TestOmarchyChoiceFixtureReplaysAllSixOperations
```

It exercises all six public mutation types through a hardened session and is
also included in the shipped strict-NDJSON example gate.

## Documentation checkpoint

The Omarchy/security material is split by audience:

- [Manual draft](manual/a2ui.md) — short user-facing staged chapter;
- [six-operation protocol guide](protocol-guide.md) — practical sequencing and fixture walkthrough;
- [security model](security-model.md) — threat boundaries and bounded safety claim;
- [security evidence](security-evidence.md) — claim → code → test → status ledger;
- [maintainer proposal draft](omarchy-submission.md) — upstream demo and submission gates;
- [`references/PROTOCOL.md`](../references/PROTOCOL.md) — normative contract.

The repository includes both the agent CLI and standard MCP facade:

```bash
make build       # Compile bin/a2ui, bin/a2uid, and bin/a2ui-mcp
make demo        # Run deterministic round-trip demo (send -> wait-event -> update)
make install     # Install all three binaries to $PREFIX/bin ($HOME/.local/bin)
```

## Diagnostics

| Symptom | Action |
| --- | --- |
| first-time setup or unclear local state | Run `a2ui doctor`; follow its `FIX:` line for each FAIL/WARN that matters to the current workflow. |
| `cannot find main module` | `cd` to the directory containing `go.mod`; do not run Go commands from the archive wrapper directory. |
| `a2ui-mcp` not found | Run `make install`, verify `command -v a2ui-mcp`, then rerun `a2ui setup-codex`. |
| Codex MCP registration missing/mismatched | Inspect `codex mcp get a2ui --json`; use `a2ui setup-codex` for a missing entry or `a2ui setup-codex --replace` only when replacement is intentional. |
| `agent stream conflict` / session already negotiated | The current MCP process cannot reconstruct the previous reliable sequence. Start a fresh daemon/session; do not retry blindly against the stale session. |
| MCP server starts but `a2ui_status` cannot reach daemon | Confirm `a2uid` is running on the `A2UI_SERVER` configured for the MCP process. |
| `a2ui_status` reports `has_client=false` | Attach the real Bubble Tea client before a human wait. |
| `no Unix socket` | Confirm the daemon is still running and copy the exact socket path it printed. |
| `ipc.client_busy` | Close the other interactive client; a second client cannot take its lease. |
| `ipc.daemon_already_running` | Use the running daemon or stop its owner; never unlink its live socket. |
| smoke daemon cannot become ready | Read the printed daemon log; commonly `SMOKE_PORT` is occupied, so choose another. |
| `make test` or `make test-race` fails | Do not claim a release pass. Record the exact failing package and test. |

## Release status

Current-head format, shipped-binary build, tests, race, vet, and fuzz smoke are
the branch CI gates. Transport smoke or focused E2E success never replaces the
full matrix, and the automated matrix never replaces the real human acceptance
gate for P0.1.

The documentation evidence ledger is authoritative for known open or
unverified security claims. A prose change must not silently promote an
`UNVERIFIED` item to a security guarantee.

Historical exact-head evidence is not merge authority for later commits. Any
new code or documentation commit requires fresh current-head verification
before reporting the branch release-green again.
