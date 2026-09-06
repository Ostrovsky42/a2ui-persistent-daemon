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
Publish the choice through A2UI, wait for my real A2UI response, then continue
based on that response and update A2UI to show the target I chose.
```

The required flow is:

```text
Codex
  -> a2ui_publish
  -> real Bubble Tea terminal UI
  -> human selects/submits in that terminal
  -> a2ui_wait_event
  -> same Codex workflow receives the semantic event
  -> Codex continues
  -> a2ui_publish updates the existing UI
```

Do not use `a2ui interact` for this acceptance test. Do not copy event JSON into Codex manually. Do not answer the choice in ordinary Codex chat.

## Tool notes

### `a2ui_publish`

The tool accepts ordered A2UI V1 operations without wire version or sequence numbers. Example shape:

```json
{
  "operations": [
    {
      "op": "upsert",
      "id": "root",
      "type": "box",
      "props": {}
    },
    {
      "op": "text",
      "id": "title",
      "text": "Choose deployment target"
    },
    {
      "op": "commit",
      "frame": "deployment-choice"
    }
  ]
}
```

The exact node topology still has to obey A2UI V1 validation. `a2ui-mcp` does not bypass the daemon Session, Document reducer, or Engine.

### `a2ui_wait_event`

Typical human wait:

```json
{
  "timeout_ms": 120000,
  "event_types": ["submit", "select"]
}
```

Non-matching events such as `committed` remain visible in `observed_events`; the tool does not silently throw them away while waiting for a matching event.

### `a2ui_status`

Use this before opening a human wait when the agent needs to know whether an interactive terminal is attached.

## P0 limitations

P0 deliberately does not provide resumable agent-stream sequencing across an `a2ui-mcp` process restart. One stdio MCP process owns one monotonic A2UI mutation sequence. If the MCP process dies while the same daemon session remains alive, start the next test with a fresh daemon/A2UI session rather than pretending the sequence can be reconstructed safely.

P0 also does not implement approval, trusted execution, capability grants, policy persistence, or A2UI Protocol V2.
