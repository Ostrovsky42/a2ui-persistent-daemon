# Agent-Connected Prototype P0 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the existing persistent A2UI daemon to real MCP-capable harnesses through a standard stdio server with publish, wait-event, and status tools.

**Architecture:** Add a long-lived `agentclient.Client` that owns only A2UI transport continuity (hello + reliable mutation sequence), then bind it to an `a2ui-mcp` stdio server built with the official MCP Go SDK. Existing Session/Engine/EventBroker remain the only semantic authorities.

**Tech Stack:** Go 1.24, existing A2UI V1 daemon HTTP bridge, `github.com/modelcontextprotocol/go-sdk/mcp` v1.4.0, standard MCP stdio.

**Why v1.4.0:** newer official SDK releases inspected during implementation require Go 1.25. The repository stays on Go 1.24. Official SDK compatibility data says v1.4.0 supports the `2025-06-18` protocol currently used by Codex's shipping legacy stdio lifecycle, plus earlier/later legacy revisions in that SDK generation. Real Codex remains the final compatibility gate.

**Spec:** `docs/superpowers/specs/2026-09-06-agent-connected-prototype-p0-design.md`

## Current status

Automated implementation is complete through packaging. Standard MCP `tools/list` and `tools/call` tests are GREEN, including two sequential publish calls preserving A2UI sequence continuity and filtered event waiting preserving observed events. Current-head full CI must still be green after the final documentation/package commits. The real Codex + real human terminal acceptance gate remains deliberately OPEN for the next hands-on session.

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

- [x] **Step 1: Add Go 1.24-compatible official MCP SDK dependency**

Pinned `github.com/modelcontextprotocol/go-sdk v1.4.0`. A first attempt with v1.7.0 produced a real toolchain incompatibility because that release requires Go 1.25; the project toolchain was not raised merely for this checkpoint.

- [x] **Step 2: Write the failing standard discovery test**

The test connects an official `mcp.Client` and server via `mcp.NewInMemoryTransports()`, calls `ListTools`, and requires exactly:

```text
a2ui_publish
a2ui_status
a2ui_wait_event
```

- [x] **Step 3: Capture real RED**

The valid tests-only RED reached compilation and failed on missing `newMCPServer`; unrelated packages remained green.

---

### Task 2: Add the long-lived A2UI agent client

**Files:**
- Create: `agentclient/client.go`
- Create: `agentclient/client_test.go`

- [x] **Step 1: Characterize sequencing**

Two calls to `Publish` require one hello and operation sequences `1,2,3,4` rather than resetting to `1` on the second call.

- [x] **Step 2: Characterize HTTP rejection**

Non-2xx daemon responses preserve status and response body in the returned error.

- [x] **Step 3: Implement minimal client**

`Publish` lazily negotiates once and sends every mutation through the existing A2UI HTTP/MCP envelope path. `agentclient` does not import `document`, `engine`, or `runtime`.

- [x] **Step 4: Add WaitEvents and Status**

`WaitEvents` uses the existing `/events` endpoint with caller context and bounded daemon timeout. `Status` decodes the existing `/status` endpoint.

---

### Task 3: Bind the three MCP tools

**Files:**
- Create: `cmd/a2ui-mcp/main.go`
- Expand: `cmd/a2ui-mcp/main_test.go`

- [x] **Step 1: Make discovery GREEN**

Exactly three P0 tools are registered with the official SDK.

- [x] **Step 2: Capture publish-call RED**

Official `tools/call` initially failed schema validation because the discovery-only server exposed empty input schemas. This proved the test was exercising the real MCP schema boundary.

- [x] **Step 3: Implement publish tool**

The adapter accepts ergonomic operations without wire sequence/version and forwards converted V1 operations to `agentclient.Client`.

- [x] **Step 4: Characterize filtered wait-event**

The fake daemon returns `committed` then `submit`. `a2ui_wait_event(event_types=["submit"])` must preserve both in `observed_events`, return the matching submit, and use one overall timeout.

- [x] **Step 5: Implement wait-event tool**

The tool loops only through the existing daemon `/events` path with the remaining deadline. No second event broker exists.

- [x] **Step 6: Implement status tool and tool errors**

Daemon errors become MCP tool errors. A normal wait with no matching event returns structured `timed_out=true`.

- [x] **Step 7: Standard MCP automated proof**

Official MCP client tests cover `tools/list` and `tools/call`. The implementation commit passed format, `go test ./...`, race, vet, and all three fuzz smoke steps before later packaging/docs commits invalidated that exact-head evidence.

---

### Task 4: Package the MCP server for Codex

**Files:**
- Modify: `Makefile`
- Create: `docs/codex-mcp.md`
- Create: `.codex/a2ui-mcp.toml.example`
- Modify: `.codex/skills/a2ui-daemon/SKILL.md`
- Modify: `docs/agent-kit.md`

- [x] **Step 1: Extend build/install/uninstall**

`make build` now produces `bin/a2ui`, `bin/a2uid`, and `bin/a2ui-mcp`; install/uninstall handle all three.

- [x] **Step 2: Document current Codex stdio configuration**

The guide uses current Codex keys: `command`, `env`, `startup_timeout_sec`, `tool_timeout_sec`, and `enabled_tools`. `command = "a2ui-mcp"` is safe when the installed binary is on Codex's inherited `PATH`; otherwise the guide requires the actual `command -v a2ui-mcp` absolute path.

- [x] **Step 3: Give Codex a validated interactive V1 example**

`docs/codex-mcp.md` uses the existing selectable-table semantics from `assets/examples/omarchy-choice.ndjson`: stable `row_ids`, `selectable=true`, action, focus, commit, then wait for `select` and consume `row_id`.

- [x] **Step 4: Update Codex skill**

The skill prefers callable MCP tools for real interaction and keeps shell/Make commands for lifecycle diagnostics.

---

### Task 5: Verification and real-human gate

- [ ] **Step 1: Require fresh current-head CI matrix**

Required on the final branch head:

```text
format
go test ./...
go test -race ./...
go vet ./...
wire fuzz smoke
document fuzz smoke
IPC fuzz smoke
```

- [x] **Step 2: Verify standard MCP directly**

Automated tests use the official MCP SDK for `tools/list` and `tools/call`, not a private JSON-RPC imitation.

- [ ] **Step 3: Real Codex + real human acceptance**

Run real Codex + real `a2uid` + real Bubble Tea client. The human must select in the terminal, the same Codex workflow must receive the event and continue, and a second `a2ui_publish` must visibly update the UI. `a2ui interact`, copied JSON, or a shell script acting as the human/agent cannot close this gate.
