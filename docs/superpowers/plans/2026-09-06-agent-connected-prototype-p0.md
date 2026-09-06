# Agent-Connected Prototype P0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the existing persistent A2UI daemon to real MCP-capable harnesses through a standard stdio server with publish, wait-event, and status tools.

**Architecture:** Add a long-lived `agentclient.Client` that owns only A2UI transport continuity (hello + reliable mutation sequence), then bind it to an `a2ui-mcp` stdio server built with the official MCP Go SDK. Existing Session/Engine/EventBroker remain the only semantic authorities.

**Tech Stack:** Go 1.24, existing A2UI V1 daemon HTTP bridge, `github.com/modelcontextprotocol/go-sdk/mcp` v1.7.0, standard MCP stdio.

**Spec:** `docs/superpowers/specs/2026-09-06-agent-connected-prototype-p0-design.md`

## Global Constraints

- Do not change A2UI `protocol.Version` from `1`.
- Do not add mutations, node types, approval semantics, or alternate semantic state.
- Standard MCP server exposes exactly `a2ui_publish`, `a2ui_wait_event`, and `a2ui_status` in P0.
- All A2UI mutations must still pass through daemon Session + Engine authority.
- One MCP process owns one monotonic A2UI reliable mutation sequence.
- Final real-Codex/human acceptance is manual and cannot be replaced by synthetic interaction.

---

### Task 1: Characterize the missing standard MCP surface

**Files:**
- Create: `cmd/a2ui-mcp/main_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

**Interfaces:**
- Consumes: official MCP Go SDK client/server test transport.
- Produces: a test that requires a `newMCPServer(*agentclient.Client) *mcp.Server` factory and exactly three tools.

- [ ] **Step 1: Add MCP SDK dependency**

Add `github.com/modelcontextprotocol/go-sdk v1.7.0` as a direct requirement and the corresponding module hashes to `go.sum`.

- [ ] **Step 2: Write the failing standard discovery test**

The test must connect an official `mcp.Client` and the future server via `mcp.NewInMemoryTransports()`, call `ListTools`, and require sorted names:

```text
a2ui_publish
a2ui_status
a2ui_wait_event
```

- [ ] **Step 3: Push tests-only RED and capture CI failure**

Expected failure: missing `newMCPServer` / missing `agentclient` implementation, not a formatting or dependency typo.

---

### Task 2: Add the long-lived A2UI agent client

**Files:**
- Create: `agentclient/client.go`
- Create: `agentclient/client_test.go`

**Interfaces:**
- Produces:
  - `func New(serverURL, sessionID string, httpClient *http.Client) *Client`
  - `func (c *Client) Publish(ctx context.Context, ops []protocol.Operation) error`
  - `func (c *Client) WaitEvents(ctx context.Context, timeout time.Duration) ([]protocol.Event, error)`
  - `func (c *Client) Status(ctx context.Context) (Status, error)`
- Client internally owns `negotiated bool` and `nextSeq uint64`, guarded by a mutex.

- [ ] **Step 1: Write failing sequencing test**

Use `httptest.Server` to record one hello and operation envelope sequences. Call `Publish` twice and require operation sequences to continue monotonically across calls, e.g. first call `1,2`, second call `3,4`.

- [ ] **Step 2: Write failing rejection test**

A non-2xx hello or operation response must return an error containing the HTTP status and daemon response body.

- [ ] **Step 3: Implement minimal client**

`Publish` lazily negotiates once, then assigns `V=protocol.Version`, `Seq=int64(nextSeq)` and forwards each operation through the existing `transport/mcp` bridge. It must not import `document`, `engine`, or `runtime`.

- [ ] **Step 4: Add WaitEvents and Status characterization tests and implementation**

`WaitEvents` GETs `/events?timeout=...` with caller context. `Status` GETs `/status` and decodes a typed struct.

---

### Task 3: Bind the three MCP tools

**Files:**
- Create: `cmd/a2ui-mcp/main.go`
- Expand: `cmd/a2ui-mcp/main_test.go`

**Interfaces:**
- `func newMCPServer(client *agentclient.Client) *mcp.Server`
- `func run(ctx context.Context, client *agentclient.Client) error`

Tool inputs/outputs:

```go
type PublishInput struct {
    Operations []OperationInput `json:"operations"`
}

type WaitEventInput struct {
    TimeoutMS  int      `json:"timeout_ms,omitempty"`
    EventTypes []string `json:"event_types,omitempty"`
}
```

Status input is an empty struct.

- [ ] **Step 1: Make discovery GREEN**

Register exactly three tools with `mcp.AddTool`.

- [ ] **Step 2: Write failing publish-call integration test**

Connect official MCP client in-memory, call `a2ui_publish`, and prove the fake daemon receives normal A2UI hello + operation envelopes rather than direct document mutation.

- [ ] **Step 3: Implement publish tool**

Convert ergonomic operation input to `protocol.Operation` without accepting caller-supplied wire version or sequence.

- [ ] **Step 4: Write failing filtered wait-event test**

Fake daemon first returns `committed`, then `submit`. `a2ui_wait_event` with `event_types=["submit"]` must continue waiting, preserve both in `observed_events`, return the matched submit event, and remain bounded by one overall timeout.

- [ ] **Step 5: Implement wait-event tool**

Loop with remaining deadline. Do not create a second daemon event queue.

- [ ] **Step 6: Implement status tool and typed tool errors**

Transport/rejection errors become MCP tool errors. No-event timeout returns structured `timed_out=true`.

- [ ] **Step 7: Run focused tests and full matrix**

Run `go test ./cmd/a2ui-mcp ./agentclient`, then `go test ./...`, race, and vet.

---

### Task 4: Package the MCP server for Codex

**Files:**
- Modify: `Makefile`
- Create: `docs/codex-mcp.md`
- Modify: `.codex/skills/a2ui-daemon/SKILL.md`

**Interfaces:**
- `make build` produces `bin/a2ui`, `bin/a2uid`, and `bin/a2ui-mcp`.
- `make install` installs all three to `$(BINDIR)`.

- [ ] **Step 1: Extend build/install/uninstall**

Add `a2ui-mcp` without changing existing binary names or targets.

- [ ] **Step 2: Document Codex stdio configuration**

Provide a concrete `~/.codex/config.toml` example:

```toml
[mcp_servers.a2ui]
command = "/home/USER/.local/bin/a2ui-mcp"
env = { A2UI_SERVER = "http://127.0.0.1:8080", A2UI_SESSION = "default" }
startup_timeout_sec = 10
tool_timeout_sec = 90
enabled_tools = ["a2ui_publish", "a2ui_wait_event", "a2ui_status"]
```

Explain that the executable path must be replaced by the actual output of `command -v a2ui-mcp`; do not ship a fake hardcoded user path in runtime config.

- [ ] **Step 3: Update Codex skill**

Prefer callable MCP tools when available. Keep shell commands for daemon diagnostics only.

---

### Task 5: Verification and tomorrow's human gate

**Files:**
- No production changes unless verification reveals a defect.

- [ ] **Step 1: Run current-head CI matrix**

Required:

```text
format
go test ./...
go test -race ./...
go vet ./...
wire fuzz smoke
document fuzz smoke
IPC fuzz smoke
```

- [ ] **Step 2: Verify standard MCP directly**

Automated tests must use official MCP `tools/list` and `tools/call`, not a private JSON-RPC imitation.

- [ ] **Step 3: Leave real-human acceptance explicitly OPEN**

Tomorrow run real Codex + real `a2uid` + real Bubble Tea client. Do not claim P0 fully closed before the same-agent human roundtrip is observed.
