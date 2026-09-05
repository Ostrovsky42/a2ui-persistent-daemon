# A2UI for Omarchy

**A2UI: Zero-sandbox ephemeral UI engine for Omarchy agents.**

A2UI gives an agent a dynamic terminal interface without requiring the agent to generate and execute a UI program first.

The bounded technical version of the pitch is:

> Agents describe interfaces with six validated UI mutations. A trusted local runtime renders them without executing generated UI code. The terminal window is ephemeral; semantic session state stays in memory while the daemon runs.

`Zero-sandbox` refers only to the UI-description plane: rendering does not require executing a generated Bash/Go/TUI program in a per-screen sandbox. It does **not** mean the agent, daemon, host actions, terminal, or user account are universally sandboxed.

## Why it fits Omarchy

Omarchy treats coding-agent CLIs as first-class tools and is deliberately terminal/TUI-heavy. A2UI adds a middle layer between plain agent text and arbitrary generated executable UI code:

```text
agent
  │ six A2UI mutations
  ▼
a2uid
  │ semantic state + user events
  ▼
a2ui
  │ Bubble Tea
  ▼
terminal
```

The terminal client can disappear and reconnect while `a2uid` keeps the semantic UI alive.

## The six public mutations

```text
upsert
props
text
remove
focus
commit
```

Those are the public UI mutation primitives. `hello`, events, telemetry, daemon/client snapshots and `frame_published` are session/transport/IPC messages, not extra mutations.

The executable reference walkthrough is:

```text
assets/examples/omarchy-choice.ndjson
```

See [Protocol guide](../docs/protocol-guide.md) for sequencing and examples. The normative source remains [`PROTOCOL.md`](PROTOCOL.md).

## Persistent state

`a2uid` owns:

- Session;
- Engine;
- authoritative Document;
- Runtime focus/input/table selection;
- Event Broker;
- registered host actions;
- publication state.

`a2ui` owns only presentation mechanics such as:

- input caret;
- viewport offset/tail pin;
- animation phase;
- terminal size.

Closing `a2ui` releases the single interactive lease but does not intentionally destroy daemon-owned semantic state. A later client receives a new atomic snapshot.

No disk persistence is promised across daemon restart or system reboot.

## Security claim

The useful comparison is deliberately narrow:

> For the task of displaying a dynamic agent-generated UI, A2UI exposes a smaller capability surface than executing arbitrary model-generated Bash with the same user privileges and no additional confinement.

The six mutations contain no shell/process/filesystem/arbitrary-network primitive. Strict decoding and candidate-based mutation also reduce parser ambiguity and partial-state corruption.

That does **not** make every A2UI deployment safe by default.

Current limitations that must remain visible in any Omarchy proposal:

- registered host actions are real privileged capabilities;
- same-UID/root processes are not isolated by Unix socket mode bits;
- the optional HTTP/MCP bridge currently has no caller-authentication layer and should be treated as loopback/dev-only;
- terminal escape/control sanitization for arbitrary agent text is not yet proven;
- retained-state limits are not a complete CPU/connection DoS proof;
- renderer publication ACK proves an exact renderer-generation path, not human attention.

The old stronger wording that implied arbitrary terminal controls were impossible has intentionally been removed until sanitizer regression evidence exists.

See:

- [Security model](../docs/security-model.md)
- [Security evidence ledger](../docs/security-evidence.md)

## Run it today

The daemon and terminal client exist now:

```bash
a2uid -session default
```

```bash
a2ui -preset dashboard
```

Default control socket:

```text
$XDG_RUNTIME_DIR/a2ui/a2ui.sock
```

Fallback is UID-scoped under `/tmp`.

Only one interactive terminal client is supported in this checkpoint.

The repository also exposes developer commands through `make help` and [`docs/agent-kit.md`](../docs/agent-kit.md).

## What is still draft

The staged Omarchy-style user chapter is:

```text
docs/manual/a2ui.md
```

It is deliberately marked draft because the project does not yet have all of the user-facing integration required for a credible upstream quickstart:

- final Omarchy package/install path;
- stable no-JSON agent CLI for publish/wait-event/status;
- clean installed demo outside the source checkout;
- terminal-control security proof;
- explicit remote HTTP access policy if remote ingress is ever supported.

Do not publish proposed commands such as `omarchy a2ui` until Omarchy integration actually implements them.

## Documentation package

This checkpoint separates audiences instead of duplicating one giant spec:

| Document | Purpose |
| --- | --- |
| [`docs/manual/a2ui.md`](../docs/manual/a2ui.md) | Short staged user chapter in Omarchy Manual style. |
| [`docs/protocol-guide.md`](../docs/protocol-guide.md) | Practical six-operation walkthrough. |
| [`references/PROTOCOL.md`](PROTOCOL.md) | Normative protocol contract. |
| [`docs/security-model.md`](../docs/security-model.md) | Threat model and bounded safety claim. |
| [`docs/security-evidence.md`](../docs/security-evidence.md) | Claim → code → test → status ledger. |
| [`docs/omarchy-submission.md`](../docs/omarchy-submission.md) | Maintainer proposal draft and submission gates. |

## Upstream posture

Current Omarchy documentation presents coding agents as first-class citizens and its newer source tree exposes the Manual under `manual/`. The exact integration path and contribution process must still be checked against the active upstream branch/release when the A2UI demo is ready.

This repository does not claim that DHH or Omarchy maintainers requested, approved, or will accept A2UI.
