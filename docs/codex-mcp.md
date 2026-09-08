# Connect A2UI to Codex

This guide is for the Agent-Connected Prototype P0.1 branch. It connects Codex to the local persistent A2UI daemon through the standard MCP stdio server `a2ui-mcp`.

The primary onboarding path is intentionally command-driven:

```text
make install
  -> a2ui setup-codex
  -> start daemon + terminal client
  -> a2ui doctor
  -> restart/reload Codex
  -> /mcp
  -> real human round-trip
```

Manual TOML editing is a fallback, not the normal setup path.

## Requirements

P0.1 targets a local single-user Unix-like development machine.

Required when building from source:

- Unix-like OS supported by the daemon/client build tags;
- Go 1.24.x;
- `make`;
- Codex CLI with `codex mcp add/get/remove` support.

Installed runtime binaries:

- `a2ui`;
- `a2uid`;
- `a2ui-mcp`.

Optional developer/smoke dependencies:

- `curl` for HTTP smoke checks;
- `jq` for JSON smoke checks;
- `socat` for IPC smoke checks.

`a2ui doctor` reports missing optional smoke tools as WARN rather than blocking the normal Codex round-trip.

Always run repository commands from the directory that actually contains this repository's `go.mod` and `Makefile`. If the checkout sits inside an outer archive/workspace directory, `cd` into the inner A2UI repository first.

## 1. Build and install

From the repository root:

```bash
make install
command -v a2ui
command -v a2uid
command -v a2ui-mcp
```

`make install` defaults to `$HOME/.local/bin`. Ensure that directory is in the `PATH` inherited by Codex.

## 2. Register A2UI in Codex

Use the A2UI setup command instead of editing Codex configuration by hand:

```bash
a2ui setup-codex \
  --server http://127.0.0.1:8080 \
  --session default
```

The command:

1. resolves the installed `codex` and `a2ui-mcp` binaries;
2. checks whether an MCP server named `a2ui` already exists;
3. refuses to overwrite an existing registration by default;
4. uses `codex mcp add` to register the stdio server;
5. verifies the saved registration with `codex mcp get a2ui --json`.

If an existing A2UI registration is intentionally being replaced:

```bash
a2ui setup-codex \
  --server http://127.0.0.1:8080 \
  --session default \
  --replace
```

Do not use `--replace` merely to silence an unexpected configuration mismatch. Inspect the existing entry first:

```bash
codex mcp get a2ui --json
```

## 3. Start a fresh persistent daemon

For the first P0.1 run, use a fresh daemon/session:

```bash
make daemon PORT=8080 SESSION=default
```

Keep it running. The command prints the Unix socket path needed by the terminal client.

One `a2ui-mcp` process owns one monotonic A2UI reliable mutation stream. P0.1 still does not reconstruct that sequence after the MCP process dies.

If a new MCP process attempts to reuse an already-negotiated daemon session, the client now fails explicitly with an agent-stream conflict and tells you to use a fresh daemon/session. Do not treat this as a retryable duplicate/no-op.

## 4. Attach the real terminal UI

In a second terminal, use the exact socket printed by `make daemon`:

```bash
make client SOCK=/tmp/a2ui-dev.EXAMPLE/a2ui.sock PRESET=dashboard
```

Replace the example socket with the real path. Only one interactive client may own the lease.

## 5. Run the preflight doctor

With the daemon and terminal running:

```bash
a2ui doctor \
  --server http://127.0.0.1:8080 \
  --session default
```

The doctor checks:

- platform;
- `a2uid` and `a2ui-mcp` availability;
- Codex CLI availability;
- Codex `a2ui` MCP registration and its command/server/session binding;
- daemon reachability and session identity;
- interactive terminal attachment;
- source/smoke dependencies.

`PASS` means the check is ready. `WARN` is non-blocking but should be understood. `FAIL` returns exit status 1 and includes a concrete `FIX:` line.

For scripts or bug reports:

```bash
a2ui doctor --json
```

## 6. Confirm tools inside Codex

Restart/reload Codex if it was already running when the MCP registration changed. In the Codex TUI use:

```text
/mcp
```

The A2UI MCP server should expose exactly:

```text
a2ui_publish
a2ui_wait_event
a2ui_status
```

Before the real human round-trip, Codex may call `a2ui_status`. A healthy attached setup reports the intended session and `has_client=true`.

## 7. Real P0.1 acceptance prompt

Use a prompt with this meaning:

```text
Use A2UI to ask me which deployment target to use: staging or production.
Do not choose for me and do not ask me in chat.
Publish a selectable A2UI table, focus it, wait for my real `select` event,
then continue based on the returned row_id and update A2UI to show the target
I chose.
```

The required flow is:

```text
Codex
  -> a2ui_publish
  -> real Bubble Tea terminal UI
  -> human selects a row and presses Enter
  -> a2ui_wait_event(event_types=["select"])
  -> same Codex workflow receives row_id=staging|production
  -> Codex continues reasoning
  -> a2ui_publish updates the existing UI
```

This gate is intentionally manual. The following do NOT count as acceptance:

- `a2ui interact` instead of the human;
- a shell script pretending to be the agent;
- copied event JSON pasted into Codex;
- answering the choice in ordinary Codex chat;
- two different Codex workflows for publish and continuation.

## Tool notes

### `a2ui_publish`

The tool accepts ordered A2UI V1 operations without caller-supplied wire version or sequence numbers. For a deployment choice, use the same selectable-table semantics covered by the repository's `omarchy-choice` fixture:

```json
{
  "operations": [
    {
      "op": "upsert",
      "id": "choice-panel",
      "type": "box",
      "parent": "root",
      "props": {"dir": "col", "gap": 1, "border": "rounded", "variant": "panel"}
    },
    {
      "op": "upsert",
      "id": "choice-title",
      "type": "text",
      "parent": "choice-panel",
      "props": {"text": "Choose a deployment target", "variant": "title"}
    },
    {
      "op": "upsert",
      "id": "targets",
      "type": "table",
      "parent": "choice-panel",
      "props": {
        "columns": [
          {"title": "Target", "width": 18},
          {"title": "State", "width": 10}
        ],
        "rows": [
          ["staging", "ready"],
          ["production", "guarded"]
        ],
        "row_ids": ["staging", "production"],
        "selectable": true,
        "action": "deployment.select",
        "variant": "compact"
      }
    },
    {"op": "focus", "id": "targets"},
    {"op": "commit", "frame": "deployment-choice"}
  ]
}
```

This remains ordinary A2UI V1. `a2ui-mcp` does not create a second choice protocol and does not bypass Session, Document reducer, Engine, or EventBroker authority.

After activation, make the agent decision from stable `row_id`, not from row position or guessed display text.

### `a2ui_wait_event`

Typical choice wait:

```json
{
  "timeout_ms": 120000,
  "event_types": ["select"]
}
```

Non-matching events such as `committed` remain visible in `observed_events`; filtering does not silently discard them.

### `a2ui_status`

Use this before opening a human wait when the agent needs to know whether the interactive terminal is attached.

## Manual Codex configuration fallback

If the installed Codex CLI does not support `codex mcp add/get/remove`, update Codex first. For diagnostic/manual fallback only, the equivalent configuration shape is:

```toml
[mcp_servers.a2ui]
command = "/absolute/path/to/a2ui-mcp"
env = { A2UI_SERVER = "http://127.0.0.1:8080", A2UI_SESSION = "default" }
startup_timeout_sec = 10
tool_timeout_sec = 130
enabled_tools = ["a2ui_publish", "a2ui_wait_event", "a2ui_status"]
```

A copyable example remains at `.codex/a2ui-mcp.toml.example`.

## Security scope

P0.1 is a local single-user prototype, not a shared-host execution boundary.

Current scope deliberately includes:

- loopback HTTP daemon access;
- no HTTP authentication;
- no approval ledger;
- no capability grants;
- no trusted-execution policy persistence.

Do not expose the daemon beyond the local trusted development machine and do not interpret human A2UI interaction as authorization for arbitrary host actions.

Approval/trusted execution remains a separate future checkpoint.

## Remaining P0.1 limitation

The one intentionally open product gate is the real Codex + real terminal + real human acceptance run. Automated MCP/daemon tests prove the integration primitives; they do not replace that same-agent human round-trip evidence.
