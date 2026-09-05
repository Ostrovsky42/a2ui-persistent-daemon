# Omarchy maintainer proposal draft

> **Status: draft for review, not an upstream submission.** Re-check the active Omarchy branch/release and contribution path immediately before filing anything upstream.

## One-line pitch

> **A2UI: Zero-sandbox ephemeral UI engine for Omarchy agents.**

Expanded technical version:

> Agents describe terminal interfaces with six bounded mutations. A trusted local runtime validates and renders them without executing generated UI code. The window is ephemeral; semantic session state remains in memory while the daemon runs.

`Zero-sandbox` here means that the **UI description plane** does not require executing a generated UI program inside a per-screen sandbox. It does not mean the agent, daemon, host actions, or user account are universally sandboxed.

## Why this fits Omarchy

Current Omarchy treats coding agents as first-class terminal tools and leans heavily on keyboard-driven terminal/TUI workflows. A2UI adds a missing primitive between "agent prints text" and "agent executes a generated program": a bounded, stateful terminal UI description channel.

The intended interaction is:

```text
agent proposes choices
        ↓
A2UI terminal panel
        ↓
user selects / types
        ↓
semantic event returns to agent
        ↓
agent updates the same panel
```

The terminal window can close and reopen without killing the semantic UI session because `a2uid` owns the long-lived state.

## What exists today

Implemented in the A2UI repository:

- persistent `a2uid` daemon;
- thin Bubble Tea `a2ui` client;
- one interactive client lease;
- Unix-domain-socket snapshot/interaction IPC;
- daemon-owned Document, focus, input values, table selection and publication state;
- reconnect with semantic state preserved;
- exact-generation renderer publication acknowledgement;
- six public mutations: `upsert`, `props`, `text`, `remove`, `focus`, `commit`;
- strict JSON decoding and transactional Document mutation;
- finite retained-state/resource limits;
- developer smoke/reattach/startup-stress harness;
- protocol, implementation and security documentation.

## What does not exist yet

Do not pitch these as finished:

- final Omarchy package/install integration;
- `omarchy a2ui` command;
- Omarchy hotkey/menu integration;
- a stable no-JSON agent-facing CLI for publish/wait-event/status;
- systemd user service/socket activation;
- Waybar integration;
- D-Bus notifications;
- ANSI/Base16 system-theme mapping;
- Wayland clipboard integration;
- Vim `hjkl` navigation;
- terminal-control sanitization proof;
- authenticated remote HTTP ingress;
- disk persistence across daemon restart.

## Demo required before submission

The maintainer-facing demo should run on a clean selected Omarchy version and show a real three-process flow:

```text
agent script / CLI
       │
       ▼
     a2uid
       │
       ▼
     a2ui
```

Required scenario:

1. start `a2uid`;
2. agent publishes a two-row choice table plus optional comment input;
3. user selects a row;
4. agent receives the semantic `select` event;
5. agent updates status/log/progress in the same UI;
6. user submits a comment;
7. close the terminal client while daemon remains alive;
8. reopen `a2ui` and show preserved semantic state;
9. finish with a `commit` and corresponding renderer publication event.

The demo must not require the reviewer to hand-write JSON or copy a generated temporary socket path between shells.

Until the user-facing agent CLI exists, this demo gate is **open**.

## Protocol summary

Public mutation vocabulary:

| Mutation | Purpose |
| --- | --- |
| `upsert` | Create a node or fully replace existing props. |
| `props` | Shallow top-level property patch. |
| `text` | Append retained content to `text`/`viewport`. |
| `remove` | Remove a node/subtree. |
| `focus` | Change semantic focus. |
| `commit` | Request renderer publication acknowledgement for the current mutation boundary. |

`hello`, events, telemetry, renderer snapshots, and `frame_published` are protocol/session/IPC messages, not extra UI mutations.

The executable reference fixture is:

```text
assets/examples/omarchy-choice.ndjson
```

## Security positioning

The maintainer proposal should make only the bounded comparison:

> For displaying a dynamic agent-generated interface, A2UI exposes a smaller capability surface than executing arbitrary model-generated Bash with the same user privileges and no additional confinement.

Evidence supporting that comparison:

- six fixed UI mutations contain no shell/process/filesystem/network primitive;
- strict JSON rejects duplicate keys and unknown fields;
- invalid candidate mutations leave authoritative Document state unchanged;
- retained state has finite limits;
- local daemon/client IPC is UID-local by Unix permissions under normal assumptions;
- unknown host action IDs are denied;
- stale renderer ACKs cannot publish newer generations.

Important limitations to state explicitly:

- registered host actions are privileged capabilities;
- same-UID/root processes are outside UDS permission isolation;
- HTTP ingress currently has no caller-authentication layer and should remain loopback/dev-only;
- terminal escape/control sanitization is not yet proven;
- renderer ACK is not proof of human attention;
- ordinary A2UI inputs are not a secrets vault;
- retained-state limits are not a complete CPU/connection DoS proof.

See `docs/security-model.md` and `docs/security-evidence.md`.

## Dependencies and operational cost

Current runtime:

- Go daemon and client;
- Bubble Tea + Lip Gloss in the terminal adapter;
- Unix domain socket for local renderer control;
- optional MCP/HTTP bridge for development/integration;
- no database required;
- no disk persistence required for the current ephemeral-session model.

Before upstream proposal, measure and publish:

```text
idle daemon RSS
attached-client RSS
startup time
reattach time
snapshot size for demo
CPU while idle
CPU during streamed text
binary sizes
```

Do not substitute theoretical estimates for measurements.

## Suggested Omarchy integration shape

If maintainers want A2UI integrated, keep the semantic daemon as the system primitive and attach Omarchy UX around it:

```text
Omarchy agent launcher / skill
          ↓
        a2uid
          ↓
terminal a2ui renderer
```

Later integrations such as theme, bar status, notifications and clipboard should attach to the daemon. The Bubble Tea window should remain disposable rather than becoming a second system service.

## Manual placement

The current Omarchy Quattro source exposes the Manual under `manual/` and its AI chapter describes coding agents as first-class citizens. The exact upstream filename/location for an A2UI page should be chosen from the active upstream tree at submission time, not hard-coded here.

Potential editorial homes:

- AI, if A2UI is treated as an agent interaction primitive;
- TUIs, if it is shipped primarily as a terminal application;
- a dedicated page only after install/launch integration exists.

This repository stages its standalone chapter at:

```text
docs/manual/a2ui.md
```

## Submission sequence

Recommended order:

1. close the no-JSON agent CLI prerequisite;
2. run the full end-user demo on a clean Omarchy install;
3. close or explicitly scope terminal-control and HTTP access gaps;
4. record fresh full test/race/vet/fuzz results on exact SHA;
5. publish resource measurements;
6. prepare a short demo recording/screenshots;
7. open an upstream proposal/issue/discussion for the architecture-sized addition before assuming a Manual PR will be accepted;
8. only then adapt the staged Manual text and packaging/integration to the path requested by maintainers.

## Evidence package to attach

- repository + exact SHA;
- demo video/GIF and reproduction commands;
- `docs/manual/a2ui.md`;
- `docs/protocol-guide.md`;
- `docs/security-model.md`;
- `docs/security-evidence.md`;
- executable `assets/examples/omarchy-choice.ndjson` fixture;
- CI URL showing `test`, `race`, `vet` and fuzz gates;
- measured resource table;
- known limitations and maintenance owner.

## Maintainer-facing summary

Suggested short proposal text after all gates are closed:

> A2UI is a small persistent terminal UI runtime for coding agents. Instead of asking an agent to generate and execute a UI program, the agent sends six validated state mutations to a local daemon. A Bubble Tea client can disappear and reconnect while the daemon keeps the semantic UI and returns user selections/input as events. The proposal is intentionally narrower than a general agent sandbox: host actions remain explicit capabilities, and the attached security evidence lists the current trust boundaries and open gaps. The demo and protocol fixture are reproducible without LLM credentials.

Do not submit that paragraph as a claim of Omarchy endorsement. It is a proposal for maintainer review.
