# Omarchy maintainer proposal draft

> **Status: draft for review, not an upstream submission.** Re-check the active Omarchy branch/release and contribution path immediately before filing anything upstream.

## One-line pitch

> **A2UI: Zero-sandbox ephemeral UI engine for Omarchy agents.**

Expanded technical version:

> Agents describe terminal interfaces with six bounded mutations. A trusted local runtime validates and renders them without executing generated UI code. The window is ephemeral; semantic session state remains in memory while the daemon runs.

`Zero-sandbox` here means that the **UI description plane** does not require executing a generated UI program inside a per-screen sandbox. It does not mean the agent, daemon, host actions, or user account are universally sandboxed.

## Evidence source of truth

This proposal deliberately does **not** duplicate changing implementation verdicts such as “verified”, “open”, “CLI ready”, “sanitizer proven”, or “full CI green”. Those statements belong in one place:

```text
docs/security-evidence.md
```

Evidence pin used while preparing this draft:

```text
A2UI integration base (develop):
9c40b58e7ed27264da8b02f51e4150072f3ce3eb

security-evidence.md content blob at that base:
e7501f67c0e28523e9c3e4a522b95b7e9527f605
```

The current Adaptive Data Surfaces R1 branch is renderer-only and is stacked from that integration base. Before an upstream Omarchy submission, replace the pin with the **final candidate commit**, rerun the ledger’s required verification commands on that exact commit, and attach the resulting CI URL. If this proposal and the ledger disagree about current security/release status, the ledger wins.

This file therefore describes architecture, value, demo acceptance criteria, integration shape and maintainer cost. It does not maintain a second release-status database in prose.

## Why this fits Omarchy

Omarchy treats coding agents as first-class terminal tools and leans heavily on keyboard-driven terminal/TUI workflows. A2UI adds a primitive between “agent prints text” and “agent executes a generated program”: a bounded, stateful terminal UI description channel.

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

The terminal window can close and reopen without requiring the semantic UI lifetime to be owned by that window; the daemon is the semantic owner.

## Stable architecture surface

The proposal is built around these repository surfaces:

- persistent `a2uid` daemon;
- Bubble Tea `a2ui` renderer client;
- daemon-owned Document, Runtime projection, events and publication state;
- Unix-domain-socket daemon/client control channel;
- six public mutations: `upsert`, `props`, `text`, `remove`, `focus`, `commit`;
- strict protocol/session/document layers;
- renderer-local presentation state;
- developer/conformance/evidence tooling and documentation.

Whether a particular security property, CLI workflow, packaging target or end-to-end demo is currently release-proven is **not restated here**. Read the pinned `docs/security-evidence.md` and the final candidate CI instead.

## Explicit non-claims

Do not pitch any of the following as part of A2UI merely because they are plausible Omarchy integrations:

- `omarchy a2ui` command;
- Omarchy hotkey/menu integration;
- systemd user service/socket activation;
- Waybar integration;
- D-Bus notifications;
- ANSI/Base16 system-theme mapping;
- Wayland clipboard integration;
- Vim `hjkl` navigation;
- authenticated remote ingress unless the evidence ledger says it is implemented and verified;
- disk persistence across daemon restart unless a later checkpoint explicitly adds it.

## Demo acceptance criteria

Before submission, run the maintainer-facing demo on a clean selected Omarchy version using the **actual installed command surface documented by the final candidate**. Do not preserve proposed command names here.

The demo must show a real three-process flow:

```text
agent-facing command/script
       │
       ▼
     a2uid
       │
       ▼
     a2ui
```

Required behavior:

1. start the daemon using the shipped installation path;
2. publish a two-row choice table plus optional comment input without hand-writing protocol JSON;
3. let the user select a row;
4. return the semantic `select` event to the agent-facing side;
5. update status/log/progress in the same semantic UI;
6. submit a comment and return its semantic event;
7. close the terminal client while the daemon remains alive;
8. reopen the renderer and demonstrate the documented semantic-state continuity;
9. finish with a `commit` and the implementation’s corresponding publication acknowledgement/event;
10. repeat from a directory outside the source checkout.

The demo must not require the reviewer to copy generated temporary socket paths between shells or know repository-internal worktree layout.

The **current pass/fail state of this demo is recorded only in the evidence ledger/final release evidence**, not in this proposal.

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

Normative semantics remain in `references/PROTOCOL.md`; the practical walkthrough is `docs/protocol-guide.md`.

## Security positioning

The maintainer proposal should make only the bounded comparison:

> For displaying a dynamic agent-generated interface, A2UI exposes a smaller capability surface than executing arbitrary model-generated Bash with the same user privileges and no additional confinement.

Do not copy the current security-status matrix into this document. The authoritative review package is:

```text
docs/security-model.md      threat model and bounded claim
docs/security-evidence.md   claim -> implementation -> regression -> status
```

This prevents proposal prose from silently retaining a stale verdict after implementation, tests, ingress policy or dependencies change.

The proposal must also preserve the distinction between:

```text
renderer publication acknowledgement != proof of human attention
registered host action              != harmless UI description
local Unix permissions              != universal process isolation
finite retained-state limits        != complete DoS proof
```

For the current status of terminal-control handling, HTTP/MCP access policy, action cancellation, UDS assumptions and release gates, cite the pinned ledger rather than paraphrasing it here.

## Dependencies and operational cost

Current architecture uses:

- Go daemon and clients/tools;
- Bubble Tea + Lip Gloss in the terminal adapter;
- Unix domain socket for local renderer control;
- transport adapters around the A2UI semantic protocol;
- no database required for the current in-memory session model.

Before upstream proposal, measure and publish on the final candidate:

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

The exact upstream filename/location for an A2UI page must be chosen from the active Omarchy tree at submission time, not hard-coded into this repository.

Potential editorial homes:

- AI, if A2UI is treated as an agent interaction primitive;
- TUIs, if it is shipped primarily as a terminal application;
- a dedicated page only after install/launch integration is accepted.

This repository stages its standalone chapter at:

```text
docs/manual/a2ui.md
```

## Submission sequence

Recommended order, expressed as gates rather than cached status claims:

1. select the exact A2UI candidate commit and Omarchy release/branch;
2. rerun every required gate from `docs/security-evidence.md` on that exact commit;
3. run the no-handwritten-JSON end-user round trip from a clean install outside the checkout;
4. run reconnect/publication behavior exactly as documented by that candidate;
5. record unresolved security limitations from the ledger without softening them in proposal prose;
6. publish resource measurements;
7. record a short demo and capture reproduction commands;
8. open the architecture-sized upstream proposal/issue/discussion before assuming a Manual PR will be accepted;
9. adapt the staged Manual text and packaging/integration only to the path maintainers request.

## Evidence package to attach

- repository + exact final commit;
- `docs/security-evidence.md` from that same commit;
- CI URL for the ledger gates;
- demo video/GIF and reproduction commands;
- `docs/manual/a2ui.md`;
- `docs/protocol-guide.md`;
- `docs/security-model.md`;
- executable `assets/examples/omarchy-choice.ndjson` fixture;
- measured resource table;
- explicit known limitations and maintenance owner.

## Maintainer-facing summary

Suggested short proposal text **only after the final evidence gate is refreshed**:

> A2UI is a small persistent terminal UI runtime for coding agents. Instead of asking an agent to generate and execute a UI program, the agent sends six validated state mutations to a local daemon. A Bubble Tea client can disappear and reconnect while the daemon keeps the semantic UI and returns user selections/input as events. The proposal is intentionally narrower than a general agent sandbox: host actions remain explicit capabilities, and the attached security evidence lists the current trust boundaries and open gaps. The demo and protocol fixture are reproducible without LLM credentials.

Do not submit that paragraph as a claim of Omarchy endorsement. It is a proposal for maintainer review.
