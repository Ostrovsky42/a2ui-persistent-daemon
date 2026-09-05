# A2UI Developer Harness and Agent Kit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Provide one safe Make-based local workflow for the persistent daemon and thin Codex/Claude operating kits.

**Architecture:** `Makefile` exposes the stable interface and delegates every runtime action to `scripts/dev/a2ui`. The script owns temporary daemon startup, readiness, logs, traps, and HTTP/IPC smoke checks. Human and provider-specific documentation invoke Make targets only.

**Tech Stack:** GNU Make, POSIX shell (`bash`), Go 1.24+, curl, jq, socat.

**Spec:** `docs/superpowers/specs/2026-09-05-a2ui-developer-agent-kit-design.md`

## Global Constraints

- Work from the directory containing `go.mod`.
- Invoke Go with `-buildvcs=false` for archive-safe local commands.
- Smoke targets use a new `0700` temporary socket directory and remove only resources they created.
- A caller-provided `SOCK` is never removed.
- Keep the existing Bubble Tea test failures visible; do not weaken renderer tests or claim a release pass.

---

### Task 1: Developer command surface

**Files:**
- Create: `Makefile`
- Create: `scripts/dev/a2ui`
- Test: manual `make help`, `make smoke-http`, `make smoke-ipc`

**Interfaces:**
- Produces: targets `daemon`, `client`, `smoke-http`, `smoke-ipc`, `test`, `test-race`, `test-reattach`, and `test-startup-stress`.
- Produces: `scripts/dev/a2ui <command>` where command is `daemon`, `client`, `smoke-http`, or `smoke-ipc`.

- [ ] **Step 1: Establish the failing command surface**

Run: `make smoke-http`

Expected: FAIL because no Makefile exists.

- [ ] **Step 2: Add the Makefile delegations**

Create variables `PORT ?= 8080`, `SESSION ?= smoke`, `PRESET ?= dashboard`, and `SOCK ?=`. Pass each through to the script. Define all verification targets with `go test` commands named in the spec.

- [ ] **Step 3: Implement isolated daemon lifecycle**

In `scripts/dev/a2ui`, use `set -euo pipefail`; require the command-specific binaries; create `mktemp -d`, `chmod 700`, start a `go run -buildvcs=false ./cmd/a2uid` child, wait for both socket and HTTP readiness, and trap exit to terminate only that child and remove only the created directory.

- [ ] **Step 4: Implement protocol assertions**

For HTTP, use the MCP headers and JSON-RPC records from the existing daemon test: assert `a2ui/hello_ack`, then require operation status `204`. For IPC, send `{"v":1,"kind":"hello","client":"dev-smoke"}` through `socat`, collect two records, and assert `hello_ack` plus `snapshot` with `jq`.

- [ ] **Step 5: Verify the harness**

Run: `make help && make smoke-http && make smoke-ipc`

Expected: all PASS; temporary daemon processes and socket directories are absent after each smoke command.

- [ ] **Step 6: Commit**

```bash
git add Makefile scripts/dev/a2ui
git commit -m "feat: add persistent daemon developer harness"
```

### Task 2: Shared agent operating contract

**Files:**
- Create: `docs/agent-kit.md`
- Modify: `README.md`
- Test: `make help`

**Interfaces:**
- Consumes: Make targets from Task 1.
- Produces: one provider-neutral operating contract.

- [ ] **Step 1: Write the contract**

Document module-root discovery, `make smoke-http` and `make smoke-ipc` success signals, foreground daemon plus second-terminal client usage, focused test targets, and diagnostics for wrong root, missing socket, busy client, and full-gate failure.

- [ ] **Step 2: Link the contract from README**

Add a short “Developer and agent workflow” section pointing to `docs/agent-kit.md` and `make help`; do not duplicate commands.

- [ ] **Step 3: Verify documented entrypoints**

Run: `make help && make test-reattach && make test-startup-stress`

Expected: help lists every target and both focused tests pass.

- [ ] **Step 4: Commit**

```bash
git add docs/agent-kit.md README.md
git commit -m "docs: add shared agent operating contract"
```

### Task 3: Provider-specific thin kits

**Files:**
- Create: `.codex/skills/a2ui-daemon/SKILL.md`
- Create: `.claude/skills/a2ui-daemon/SKILL.md`
- Test: `test -f` checks and duplicate-contract scan

**Interfaces:**
- Consumes: `docs/agent-kit.md` and Make targets from Task 1.
- Produces: Codex and Claude discovery documents without protocol duplication.

- [ ] **Step 1: Add Codex kit**

State the purpose, direct the agent to read `docs/agent-kit.md`, require `make smoke-http` and `make smoke-ipc` before reporting transport success, and prohibit a release claim when full gates fail.

- [ ] **Step 2: Add Claude kit**

Use the same behavioral contract and Make commands, changing only provider metadata or discovery wording required by Claude.

- [ ] **Step 3: Check kits remain thin**

Run: `rg -n 'jsonrpc|MCP-Protocol-Version|UNIX-CONNECT' .codex .claude`

Expected: no protocol payload or socket implementation appears in either provider kit.

- [ ] **Step 4: Commit**

```bash
git add .codex/skills/a2ui-daemon/SKILL.md .claude/skills/a2ui-daemon/SKILL.md
git commit -m "feat: add Codex and Claude daemon kits"
```

### Task 4: Final tooling verification

**Files:**
- Modify: none unless a Task 1–3 acceptance condition fails.

- [ ] **Step 1: Run all new focused checks**

Run: `make smoke-http && make smoke-ipc && make test-reattach && make test-startup-stress && go vet ./...`

Expected: PASS.

- [ ] **Step 2: Run release gates without masking failures**

Run: `make test; make test-race`

Expected: report the exact observed outcome; retain known renderer failures if still present.

- [ ] **Step 3: Inspect worktree**

Run: `git status --short && git log --oneline -4`

Expected: only the three task commits plus the pre-existing baseline/spec commits; no temporary sockets, binaries, or logs are tracked.
