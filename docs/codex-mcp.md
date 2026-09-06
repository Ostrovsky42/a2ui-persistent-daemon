# Connect A2UI to Codex

This guide is for the Agent-Connected Prototype P0 branch. It connects Codex to the local persistent A2UI daemon through the standard MCP stdio server `a2ui-mcp`.

## 1. Build and install

From the repository root:

```bash
make install
command -v a2ui-mcp
command -v a2uid
command -v a2ui
```

`make install` defaults to `$HOME/.local/bin`. If `command -v a2ui-mcp` prints nothing, add that directory to `PATH` before launching Codex or use the absolute binary path in the Codex config.

## 2. Start the persistent daemon

In terminal 1:

```bash
make daemon PORT=8080 SESSION=default
```

Keep the daemon running. It prints the Unix socket path used by the interactive client.

For the first P0 run, use a fresh daemon/session. The `a2ui-mcp` process owns the reliable mutation sequence for that session and P0 does not yet reconstruct it after an MCP-process restart.

## 3. Attach the real terminal UI

In terminal 2, use the exact socket printed by the daemon:

```bash
make client SOCK=/tmp/a2ui-dev.EXAMPLE/a2ui.sock PRESET=dashboard
```

Replace the example socket with the real path. Only one interactive client may hold the lease.

## 4. Register the MCP stdio server in Codex

Codex supports local stdio MCP servers through `[mcp_servers.<id>]`. Add the following to `~/.codex/config.toml`:

```toml
[mcp_servers.a2ui]
command = "a2ui-mcp"
env = { A2UI_SERVER = "http://127.0.0.1:8080", A2UI_SESSION = "default" }
startup_timeout_sec = 10
tool_timeout_sec = 130
enabled_tools = ["a2ui_publish", "a2ui_wait_event", "a2ui_status"]
```

A copyable version is also stored at `.codex/a2ui-mcp.toml.example`.

If `a2ui-mcp` is not on the `PATH` inherited by Codex, replace `command` with the absolute path printed by:

```bash
command -v a2ui-mcp
```

The 130-second Codex tool timeout is deliberately slightly larger than P0's maximum `a2ui_wait_event` timeout of 120 seconds.

Restart/reload Codex after changing its MCP configuration. Verify the server is registered with the Codex MCP UI/command surface available in your installed version; the expected tool names are exactly:

```text
a2ui_publish
a2ui_wait_event
a2ui_status
```

## 5. Check the daemon before the human round-trip

Ask Codex to call `a2ui_status`. A healthy attached setup should report:

```text
session: default
has_client: true
```

The exact revision/node/generation values depend on current daemon state.

If `has_client` is false, attach the Bubble Tea client before testing human interaction.

## 6. Real P0 acceptance prompt

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
  -> Codex continues
  -> a2ui_publish updates the existing UI
```

Do not use `a2ui interact` for this acceptance test. Do not copy event JSON into Codex manually. Do not answer the choice in ordinary Codex chat.

## Tool notes

### `a2ui_publish`

The tool accepts ordered A2UI V1 operations without wire version or sequence numbers. For a deployment choice, use the same selectable-table semantics already covered by the repository's `omarchy-choice` fixture:

```json
{
  "operations": [
    {
      "op": "upsert",
      "id": "choice-panel",
      "type": "box",
      "parent": "root",
      "props": {
        "dir": "col",
        "gap": 1,
        "border": "rounded",
        "variant": "panel"
      }
    },
    {
      "op": "upsert",
      "id": "choice-title",
      "type": "text",
      "parent": "choice-panel",
      "props": {
        "text": "Choose a deployment target",
        "variant": "title"
      }
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
    {
      "op": "focus",
      "id": "targets"
    },
    {
      "op": "commit",
      "frame": "deployment-choice"
    }
  ]
}
```

This is deliberately ordinary A2UI V1. `a2ui-mcp` does not create a second choice protocol and does not bypass the daemon Session, Document reducer, or Engine.

After a row is activated, the semantic event contains `ev="select"`, the table ID, the action, row index, and stable `row_id`. The agent should make the decision from `row_id`, not infer it from visual position.

### `a2ui_wait_event`

For the choice above:

```json
{
  "timeout_ms": 120000,
  "event_types": ["select"]
}
```

Non-matching events such as `committed` remain visible in `observed_events`; the tool does not silently throw them away while waiting for a matching event.

### `a2ui_status`

Use this before opening a human wait when the agent needs to know whether an interactive terminal is attached.

## P0 limitations

P0 deliberately does not provide resumable agent-stream sequencing across an `a2ui-mcp` process restart. One stdio MCP process owns one monotonic A2UI mutation sequence. If the MCP process dies while the same daemon session remains alive, start the next test with a fresh daemon/A2UI session rather than pretending the sequence can be reconstructed safely.

P0 also does not implement approval, trusted execution, capability grants, policy persistence, or A2UI Protocol V2.
