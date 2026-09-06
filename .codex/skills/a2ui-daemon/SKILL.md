---
name: a2ui-daemon
description: Use and verify the local A2UI persistent daemon, callable MCP tools, and Unix IPC workflow. Use for human-facing A2UI interaction and a2uid/a2ui diagnostics; not for changing the public protocol.
---

# A2UI persistent daemon

Work from the Go module root, containing `go.mod` and `Makefile`. Read
`docs/agent-kit.md` for daemon operations and `docs/codex-mcp.md` for the real
Codex MCP connection flow.

When the registered MCP tools are available, prefer them over shell commands for
agent-to-human interaction:

- `a2ui_status` checks daemon/session state and whether a real interactive client is attached;
- `a2ui_publish` publishes ordered A2UI V1 operations;
- `a2ui_wait_event` waits when continuation genuinely depends on a human semantic event.

Do not use A2UI for every minor question. Use it when a structured terminal
surface materially helps the human inspect, select, submit, or monitor state.
When a workflow depends on a human response, do not invent the answer or ask the
same choice in chat after publishing it through A2UI.

The MCP tool layer is only an adapter. Do not bypass the daemon Session, Engine,
or event broker, and do not add protocol fields to make a tool call easier.
`a2ui interact` is a deterministic test/diagnostic helper and does not count as
real-human acceptance.

Use shell/Make commands for lifecycle and diagnostics. Run both
`make smoke-http` and `make smoke-ipc` before reporting daemon transport
success. Use `make test-reattach` and `make test-startup-stress` for focused
lifecycle evidence.

Do not copy MCP request payloads, create ad-hoc socket cleanup, or bypass the
harness with a second daemon. A `client_busy` response means another terminal
owns the only interactive lease. A full release claim requires both `make test`
and `make test-race` to pass; report their fresh result rather than relying on
earlier smoke evidence.
