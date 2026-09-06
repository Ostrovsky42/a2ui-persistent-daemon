---
name: a2ui-daemon
description: Use and verify the local A2UI persistent daemon, callable MCP tools, and Unix IPC workflow. Use for human-facing A2UI interaction and a2uid/a2ui diagnostics; not for changing the public protocol.
---

# A2UI persistent daemon

Work from the actual Go module root containing `go.mod` and `Makefile`. Read
`docs/codex-mcp.md` for the user onboarding/real acceptance path and
`docs/agent-kit.md` for lower-level daemon operations.

Before diagnosing a human-facing workflow, prefer the supported preflight:

```bash
a2ui doctor
```

If the Codex MCP registration is missing, use:

```bash
a2ui setup-codex
```

Do not edit Codex MCP TOML by hand unless the documented fallback is explicitly
needed. `setup-codex` uses the Codex CLI configuration path, refuses to replace
an existing `a2ui` registration by default, and verifies the saved result.

When the registered MCP tools are available, prefer them over shell commands for
agent-to-human interaction:

- `a2ui_status` checks daemon/session state and whether a real interactive client is attached;
- `a2ui_publish` publishes ordered A2UI V1 operations;
- `a2ui_wait_event` waits when continuation genuinely depends on a human semantic event.

Do not use A2UI for every minor question. Use it when a structured terminal
surface materially helps the human inspect, select, submit, or monitor state.
When a workflow depends on a human response, do not invent the answer or ask the
same choice in chat after publishing it through A2UI.

For selectable tables, continue from stable `row_id`, not row position or
guessed display text.

The MCP tool layer is only an adapter. Do not bypass the daemon Session, Engine,
or EventBroker, and do not add protocol fields to make a tool call easier.
`a2ui interact` is a deterministic test/diagnostic helper and does not count as
real-human acceptance.

One P0.1 MCP process owns one reliable mutation sequence. If publishing fails
with an agent-stream conflict because the daemon session was already negotiated
by a previous MCP process, do not retry blindly against the same session. Start
a fresh daemon/session as directed by the diagnostic.

Use shell/Make commands for lifecycle and lower-level diagnostics. Run both
`make smoke-http` and `make smoke-ipc` before reporting daemon transport
success. Use `make test-reattach` and `make test-startup-stress` for focused
lifecycle evidence.

Do not copy MCP request payloads, create ad-hoc socket cleanup, or bypass the
harness with a second daemon. A `client_busy` response means another terminal
owns the only interactive lease. A full release claim requires current-head
format, build, test, race, vet, and fuzz/smoke evidence; report fresh results
rather than relying on an earlier SHA.
