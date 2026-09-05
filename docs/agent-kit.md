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

`make test` and `make test-race` remain release gates. At the current
baseline, Bubble Tea renderer failures are known in those full gates. The
agent must report them plainly; smoke or focused E2E success does not replace
a full release verification.
