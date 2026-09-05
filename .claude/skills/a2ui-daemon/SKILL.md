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

Do not copy MCP request payloads, create ad-hoc socket cleanup, or bypass the
harness with a second daemon. A `client_busy` response means another terminal
owns the only interactive lease. A full release claim requires both `make test`
and `make test-race` to pass; report their fresh result rather than relying on
earlier smoke evidence.
