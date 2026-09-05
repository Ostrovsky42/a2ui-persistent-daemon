# A2UI v1 transport profiles

A2UI semantics are transport-neutral. This document defines what each profile may carry and what guarantees it MUST provide.

## 1. NDJSON / stdio

Use for local agents, subprocesses, logging, fixtures and record/replay.

- one JSON record per line;
- empty lines ignored;
- newline framing is not counted against JSON record-size budget;
- malformed JSON line yields a recoverable wire error and the next line may still be read;
- reader/I/O failure is distinct from clean EOF;
- duplicate object keys and unknown top-level fields are rejected.

NDJSON is the canonical human-debuggable codec, not the semantic protocol itself.

## 2. HTTP streaming / HTTP/2

`transport/httpstream` is an ordered request-body/response-body NDJSON streaming handler using Go `net/http`.

Each accepted input record is handled in order and its response record is written and flushed immediately.

Recommended production profile:

- TLS;
- HTTP/2 enabled;
- application-level A2UI sequence validation;
- server/client timeouts;
- transport authentication outside A2UI core;
- bounded A2UI record sizes.

HTTP/2 supplies framing, multiplexing, reliable byte transport and transport flow control. A2UI does not reimplement them.

A malformed record before any response bytes produces HTTP 400. If corruption is discovered after streaming has begun, the stream terminates; HTTP status can no longer be rewritten safely.

## 3. MCP bridge — target 2026-07-28

Modern MCP is stateless at protocol transport level. A2UI therefore does not depend on an MCP session header/handshake.

A2UI state identity travels explicitly in `Envelope.Session`. The bridge maps envelope kinds to JSON-RPC methods:

```text
a2ui/hello
a2ui/hello_ack
a2ui/operation
a2ui/event
a2ui/telemetry
```

For modern Streamable HTTP integration helpers emit:

```text
MCP-Protocol-Version: 2026-07-28
Mcp-Method: <JSON-RPC method>
```

Header/body disagreement is rejected.

The `a2ui/*` method namespace in this repository is an integration convention, not an assertion of registration in the MCP specification. Hosts that require strict official MCP surfaces should expose A2UI through an approved tool/extension policy while preserving A2UI envelopes unchanged as application payload.

## 4. UDP telemetry profile

UDP is not a reliable A2UI document transport.

Allowed:

- transient telemetry;
- meters/sensor samples;
- non-authoritative visualization samples.

Forbidden:

- `operation` envelopes;
- document create/update/remove/focus/commit;
- submit;
- action invocation/results;
- reliable errors/commit acknowledgements.

Datagram header contains:

- magic `A2UI`;
- A2UI version;
- explicit session handle;
- `telemetry` channel;
- monotonically increasing datagram sequence;
- payload.

One A2UI message MUST fit in one datagram. A2UI v1 does not fragment/reassemble. Default codec limit is 1200 bytes to reduce accidental IP fragmentation risk; deployments may negotiate a lower value.

Sequence permits receivers to observe loss/reordering/duplicates; it does not imply retransmission.

If reliable remote UI mutation over datagrams is required, use QUIC or another reliable transport rather than extending this UDP profile into a home-grown transport protocol.

## 5. Host-local daemon/client IPC

This profile is **not** an Agent-facing A2UI transport. It is an internal trusted-host control protocol between persistent `a2uid` and a Bubble Tea `a2ui` client. Public A2UI envelopes continue to arrive through NDJSON/HTTP/MCP as before.

Profile:

- Unix domain stream socket only;
- strict one-JSON-value-per-line framing;
- independent IPC version `1`;
- default maximum record size 16 MiB;
- default path `$XDG_RUNTIME_DIR/a2ui/a2ui.sock`, fallback `/tmp/a2ui-$UID/a2ui.sock`;
- parent directory `0700`; socket and startup lock `0600`;
- exactly one interactive client lease;
- bounded/coalesced daemon snapshot notification;
- serialized socket writer;
- no remote/TCP exposure.

Client messages are `hello`, `interaction`, `frame_published`, and `detach`. Daemon messages are `hello_ack`, `snapshot`, and `error`. Semantic interactions map to existing Engine APIs (`focus`, `input_set`, `input_submit`, `table_move`, `table_activate`, `action_key`). Caret movement, viewport scroll offset, animation phase and terminal resize remain adapter-local and are not sent over IPC.

A `snapshot` contains one atomic `Engine.PresentationSnapshot`; it is not encoded as a recursive public `upsert`. A pending commit remains pending until the client acknowledges the exact rendered publication generation. Snapshot transmission, socket write completion, or absence of a client never substitute for this ACK.
