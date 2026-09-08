# P0.1 Real Codex + Human Acceptance

Status: **OPEN until observed with a real Codex process and a real Bubble Tea terminal client.**

This gate is deliberately manual. Automated tests prove the transport and tool contracts but cannot prove that a human actually interacted with the real terminal surface and that the originating Codex workflow continued from that event.

## Preconditions

From the A2UI repository root:

```bash
make install
a2ui setup-codex --server http://127.0.0.1:8080 --session p01-human
```

Start a fresh daemon:

```bash
make daemon PORT=8080 SESSION=p01-human
```

Attach the terminal client in another terminal using the exact socket path printed by the daemon:

```bash
make client SOCK=<printed-socket> PRESET=dashboard
```

Then require a clean preflight:

```bash
a2ui doctor --server http://127.0.0.1:8080 --session p01-human
```

Optional machine-readable capture:

```bash
a2ui doctor --server http://127.0.0.1:8080 --session p01-human --json
```

Restart/reload Codex after registration changes. In Codex, `/mcp` must show the A2UI server and these tools:

```text
a2ui_publish
a2ui_wait_event
a2ui_status
```

## Acceptance prompt

Give the real Codex workflow this task:

```text
Use A2UI to ask me which deployment target to use: staging or production.
Do not choose for me and do not ask me in chat.
Publish a selectable A2UI table, focus it, wait for my real select event,
continue based on the returned row_id, and update A2UI to show the target I chose.
```

## Required observation

The same Codex workflow must perform this sequence:

```text
Codex
  -> a2ui_publish
  -> daemon-owned A2UI document changes
  -> real Bubble Tea terminal shows staging / production
  -> human moves selection and presses Enter
  -> daemon emits semantic select event
  -> a2ui_wait_event returns that event to the originating Codex workflow
  -> Codex uses stable row_id=staging|production
  -> Codex continues
  -> second a2ui_publish updates the existing UI
```

## Evidence record

Record the following after the run. Do not pre-fill PASS values.

```text
P0.1 REAL CODEX/HUMAN ACCEPTANCE

date:
branch:
exact head:
Codex version:
OS:

doctor preflight:
  result:
  notable WARNs:

Codex /mcp:
  a2ui server visible:
  a2ui_publish visible:
  a2ui_wait_event visible:
  a2ui_status visible:

round-trip:
  first publish called by Codex:
  real terminal UI appeared:
  human selected row in terminal:
  select event returned to same Codex workflow:
  returned row_id:
  Codex continued from row_id:
  second publish updated same UI:

synthetic a2ui interact used:
manual event JSON copied into Codex:
choice answered in ordinary Codex chat:

result: PASS / FAIL
failure point:
notes:
```

## Invalid evidence

The gate remains OPEN if any of the following substitutes for the real path:

- `a2ui interact` generates the acceptance event;
- a shell/Go test acts as the agent;
- event JSON is manually copied into Codex;
- the human answers in normal Codex chat;
- one Codex session publishes and another receives the event;
- only `tools/list` or mock `tools/call` is demonstrated.

## Restart rule

P0.1 does not reconstruct reliable mutation sequencing after `a2ui-mcp` restarts. If the MCP process is restarted, use a fresh daemon/session for the next acceptance attempt. Reuse of an already-negotiated session must fail with the explicit agent-stream conflict diagnostic rather than being treated as a successful reconnect.
