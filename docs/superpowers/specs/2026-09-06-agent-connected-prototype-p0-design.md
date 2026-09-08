# Agent-Connected Prototype P0 Design

## Goal

Connect a real MCP-capable agent harness to the existing persistent A2UI daemon without changing A2UI Protocol V1. The same agent process must be able to publish UI, wait for a real human interaction from the terminal client, consume the semantic event, and publish a follow-up update.

## Hard boundary

- A2UI `protocol.Version` remains `1`.
- The six public mutations remain `upsert`, `props`, `text`, `remove`, `focus`, and `commit`.
- No new `NodeType` is added.
- No approval/capability-broker semantics are added.
- The MCP layer is an adapter and owns no Document, Runtime, or event state.
- The existing daemon `Session`, `Engine`, and `EventBroker` remain semantic authorities.

## Current gap

The repository already has a persistent daemon HTTP bridge and agent-oriented CLI commands, but `transport/mcp` is an A2UI-envelope transport mapping rather than a standard MCP tool server. The current Codex skill is operational documentation, not a callable tool surface.

A real harness therefore cannot discover and call `a2ui_publish`, `a2ui_wait_event`, and `a2ui_status` through ordinary MCP `tools/list` / `tools/call`.

## Architecture

```text
Codex / Claude / MCP client
          |
          | standard MCP over stdio
          v
      a2ui-mcp
          |
          | long-lived agentclient.Client
          v
      existing a2uid
          |
          +--> Session.AcceptMutation
          +--> Engine.Apply
          +--> EventBroker / WaitEvents
          |
          v
   existing Bubble Tea client
```

The MCP process is long-lived for one harness connection. It owns only client-side transport continuity: A2UI hello state and the next reliable mutation sequence. It does not own semantic UI state.

## Why the MCP process must be long-lived

A2UI V1 mutation sequencing belongs to one reliable agent stream. `Session.AcceptMutation` expects sequence 1, 2, 3, ... and treats lower values as duplicates. Starting an independent `a2ui send` subprocess for every MCP tool call would reset sequencing and can silently turn later updates into duplicates. Therefore the MCP server holds one `agentclient.Client` for its lifetime and advances the sequence across repeated `a2ui_publish` calls.

## Standard MCP compatibility

Use the official Go MCP SDK (`github.com/modelcontextprotocol/go-sdk/mcp`) rather than a hand-written partial protocol implementation. P0 uses stdio because it is the most portable local harness transport.

The repository remains on Go 1.24. The newest SDK releases inspected during P0 had moved to Go 1.25, so this checkpoint intentionally pins `github.com/modelcontextprotocol/go-sdk v1.4.0`, the compatible official SDK line observed to retain Go 1.24 support. We do not claim that this library version implements every newer MCP revision. Automated tests prove standard `initialize`/`tools/list`/`tools/call` interoperability through the official SDK, and the real current Codex run is the authoritative harness-compatibility gate.

The server exposes exactly three tools:

- `a2ui_publish`
- `a2ui_wait_event`
- `a2ui_status`

## Tool contracts

### a2ui_publish

Input is an ergonomic list of A2UI V1 operations without wire version or sequence fields. The adapter assigns the next reliable sequence and forwards each operation through the existing daemon HTTP bridge. It performs one A2UI hello lazily before the first mutation.

The tool must never mutate a Document or call the reducer directly.

### a2ui_wait_event

Waits on the daemon `/events` endpoint with a bounded timeout. The result preserves the original structured `protocol.Event` fields. An optional `event_types` allowlist lets the caller wait through publication acknowledgements until a human-semantic event such as `submit` or `select` appears.

Events observed while waiting are returned in the result so filtering does not silently hide what happened.

### a2ui_status

Reads the existing daemon `/status` endpoint and returns session, revision, node count, interactive-client presence, publication generation, and pending-publication state.

## Error model

Transport failure, non-2xx daemon response, invalid tool input, context cancellation, and timeout are distinguishable. Tool failures are reported as MCP tool errors; timeouts with no matching event are a successful structured result with `timed_out=true`, because the agent remains runnable and may decide what to do next.

## Lifecycle characteristics

- No terminal client: publish may succeed because semantic state is daemon-owned; `a2ui_status.has_client` tells the agent there is no attached human surface.
- Reattach: existing daemon state and publication semantics remain authoritative.
- MCP disconnect: the client-side mutation sequence owner is gone; P0 does not claim resumable agent-stream reconnection.
- New MCP process against an already-negotiated A2UI Session is outside P0 and must not be misrepresented as supported.

## Codex connection

P0 ships an `a2ui-mcp` binary, Makefile build/install targets, and a documented `~/.codex/config.toml` stanza using `[mcp_servers.a2ui]` with stdio. Project-local config is optional because Codex may require a trusted workspace before loading local MCP configuration.

## Acceptance

Automated proof covers standard MCP tool discovery and calls through the official MCP client plus persistent A2UI sequencing across multiple publish calls.

Final human acceptance remains manual:

```text
real Codex
 -> a2ui_publish
 -> real terminal UI
 -> human submit/select
 -> a2ui_wait_event returns to same Codex turn/workflow
 -> Codex continues
 -> second a2ui_publish changes the UI
```

`a2ui interact`, copied JSON, or a shell script acting as the agent does not satisfy that final gate.
