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

## Local interaction hot path

Treat the LLM/agent as the cold path and the terminal runtime as the local hot
path. Publish enough semantic data for the human to inspect the current surface
without another agent turn. Use stable `row_ids`, existing responsive layout,
and existing V1 primitives. Let the runtime own table movement, master-detail
projection, focus traversal, viewport scrolling, input editing and contextual
help.

Call `a2ui_wait_event` only when continuation genuinely depends on committed
human intent. In the terminal host, table Enter produces `select`, input Enter
produces `submit`, and an explicitly declared action hotkey may produce its
semantic action event. Arrow keys, Home/End, PageUp/PageDown, scrolling, Tab,
Shift+Tab, caret movement and master-detail inspection are not decisions and
must not wake the agent.

Do:

- publish all currently available detail fields needed for local inspection;
- keep selectable data identity stable with `row_ids`;
- use `box` row/column layout and `responsive:"stack"` instead of republishing for terminal width;
- let renderer/runtime-generated contextual help explain the actual local controls;
- wait only after a semantic boundary where the human committed intent.

Do not:

- tell the user to use arrow keys, Home/End, Tab or scrolling in Document prose;
- republish details merely because the selected row moved;
- treat selection movement as a decision;
- wake on viewport scrolling or input caret/edit operations;
- build manual navigation/help text into A2UI nodes;
- invent unsupported widgets, node types, mutations or executable navigation behavior.

See `docs/local-hot-path-recipes.md` for the five canonical V1 recipes: choice,
form, master-detail, status-dashboard and progress-log.

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
