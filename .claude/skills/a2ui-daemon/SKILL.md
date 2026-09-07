---
name: a2ui-daemon
description: Run and verify the local A2UI persistent daemon, MCP bridge, and Unix IPC workflow. Use for a2uid/a2ui development and diagnostics; not for changing the public protocol.
---

# A2UI persistent daemon

Work from the Go module root, containing `go.mod` and `Makefile`. Read
`docs/agent-kit.md` before starting or diagnosing a daemon workflow.

Use the Make targets as the only developer interface. In particular, run both
`make smoke-http` and `make smoke-ipc` before reporting daemon transport
success. Use `make test-reattach` and `make test-startup-stress` for focused
lifecycle evidence.

## Local interaction hot path

The LLM/agent is the cold path; the terminal runtime is the local hot path.
Publish enough semantic data for local inspection, prefer stable `row_ids`, and
use the existing V1 responsive layout. Table movement, master-detail projection,
focus traversal, viewport scrolling, input editing and contextual help belong to
the local runtime.

Wait for the human only at a semantic boundary: table Enter -> `select`, input
Enter -> `submit`, or an explicitly declared semantic action hotkey. Arrow keys,
Home/End, PageUp/PageDown, scrolling, Tab/Shift+Tab, caret movement and
master-detail inspection must not wake the agent.

Do not write navigation instructions into A2UI prose, republish details after
row movement, treat selection movement as a decision, wake on scrolling, build
manual help nodes, or invent unsupported widgets. See
`docs/local-hot-path-recipes.md` for the canonical choice, form, master-detail,
status-dashboard and progress-log recipes.

Do not copy MCP request payloads, create ad-hoc socket cleanup, or bypass the
harness with a second daemon. A `client_busy` response means another terminal
owns the only interactive lease. A full release claim requires both `make test`
and `make test-race` to pass; report their fresh result rather than relying on
earlier smoke evidence.
