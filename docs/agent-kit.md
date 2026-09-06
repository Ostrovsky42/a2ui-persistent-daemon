# A2UI agent operating contract

Use this document from the Go module root: the directory containing `go.mod`
and `Makefile`.

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

The HTTP smoke proves the MCP `hello` / `hello_ack` path and one operation.
The IPC smoke proves the Unix-socket `hello_ack` and atomic initial snapshot.
Neither check renders a Bubble Tea terminal.

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

The repository includes a dedicated agent CLI and round-trip verification:

```bash
make build       # Compile bin/a2ui and bin/a2uid
make demo        # Run automated round-trip demo (send -> wait-event -> update)
make install     # Install to $PREFIX/bin ($HOME/.local/bin)
```

## Diagnostics

| Symptom | Action |
| --- | --- |
| `cannot find main module` | `cd` to the directory containing `go.mod`; do not run Go commands from the archive wrapper directory. |
| `no Unix socket` | Confirm the daemon is still running and copy the exact socket path it printed. |
| `ipc.client_busy` | Close the other interactive client; a second client cannot take its lease. |
| `ipc.daemon_already_running` | Use the running daemon or stop its owner; never unlink its live socket. |
| smoke daemon cannot become ready | Read the printed daemon log; commonly `SMOKE_PORT` is occupied, so choose another. |
| `make test` or `make test-race` fails | Do not claim a release pass. Record the exact failing package and test. |

## Release status

`make test` and `make test-race` remain release gates. Smoke or focused E2E
success never replaces them.

The documentation evidence ledger is authoritative for known open or
unverified security claims. A prose change must not silently promote an
`UNVERIFIED` item to a security guarantee.

The branch-level GitHub Actions matrix was fully green on exact head
`6f1b08b6bfa5c97ec6bedf7f53df5727d804fad8` in run `33991876834`:

```text
format                         PASS
go test ./...                  PASS
go test -race ./...            PASS
go vet ./...                   PASS
wire fuzz smoke                PASS
document reducer fuzz smoke    PASS
IPC codec fuzz smoke           PASS
```

Any later code change invalidates that exact-head evidence and requires a new
full run before reporting the branch release-green again.
