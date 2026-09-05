# A2UI Persistent Daemon IPC Design

**Status:** implemented; exact Go 1.24 + upstream Bubble Tea/Lip Gloss verification remains environment-dependent.

## Goal

Separate semantic A2UI lifetime from the scratchpad terminal. `a2uid` must keep Session, Engine, Document, Runtime and event/action state alive while a single Bubble Tea client detaches and later reattaches.

## Process model

```text
Agent/MCP → a2uid(Session + Engine + Runtime) → Unix socket → a2ui(Bubble Tea)
```

Daemon is sole semantic authority. Client owns only renderer-local mechanics. `a2ui-runner` remains an in-process compatibility/dev path.

## IPC boundary

The daemon/client protocol is internal and versioned independently from public A2UI. It uses strict bounded NDJSON over a per-user Unix socket. Core message kinds: `hello`, `hello_ack`, `interaction`, `snapshot`, `frame_published`, `detach`, `error`. One maximum-sized atomic snapshot fits inside the 16 MiB IPC record budget.

## Single-client ownership

Only one valid interactive client may acquire the lease. Lease is acquired after `hello`, preventing a silent pre-handshake connection from owning semantic input. A competing client gets `ipc.client_busy`. Disconnect/EOF/detach releases ownership.

## Persistent vs local state

Persistent daemon state: Document, revision, focus, input values, table selection, Session sequencing, events/actions and publication state. Ephemeral client state: caret, viewport offset/tail pin, animation phase and terminal dimensions.

## Snapshot synchronization

Attach sends one atomic `Engine.PresentationSnapshot`. Subsequent semantic changes coalesce into latest-snapshot notifications. No second generic delta protocol is introduced. Blocking writes occur outside Engine locks and have finite write deadlines.

## Publication correctness

A pending semantic frame has a monotonically changing publication generation. Snapshot transmission does not publish. After rendering, client sends `frame_published(generation)`. Daemon only publishes an exact current pending generation. Duplicate ACK is idempotent; stale ACK cannot publish newer state. Disconnect before ACK keeps the barrier pending for reattach.

## Socket security/lifecycle

Default: `$XDG_RUNTIME_DIR/a2ui/a2ui.sock`, fallback `/tmp/a2ui-$UID/a2ui.sock`. Directory `0700`; socket/startup lock `0600`. Stale probe/remove/bind runs under a per-path advisory lock to eliminate concurrent-start TOCTOU. The UnixListener owns pathname unlink on close; the daemon never performs delayed unconditional `os.Remove(path)`.

## Failure behavior

Fatal transport/codec errors mark the client disconnected and never create a fallback Engine. Recoverable IPC diagnostics remain separate. A dead or slow client cannot grow unbounded snapshot queues or hold the Engine mutex during socket I/O.

## Deferred

Multi-client editing, disk persistence, systemd/socket activation, Waybar, D-Bus notifications, clipboard, Vim navigation, theme discovery, macro/preset protocol and public recursive snapshots are out of scope.
