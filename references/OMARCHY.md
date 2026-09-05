# A2UI for Omarchy

**A2UI: Zero-sandbox ephemeral UI engine for Omarchy agents.**

A2UI lets an AI agent build a native terminal interface without asking the agent to generate and execute a program first.

The agent sends six bounded UI operations. A trusted local runtime validates them, keeps the interface state, and renders it with Bubble Tea. Close the terminal window and the daemon keeps the semantic UI alive. Open `a2ui` again and the current screen comes back.

That fits the Omarchy model well: agents are first-class tools, terminal applications are normal desktop applications, and persistent processes can outlive the window used to view them.

## Run it

Start the persistent runtime:

```bash
a2uid -session default
```

Open the interface:

```bash
a2ui -preset dashboard
```

Other renderer presets:

```bash
a2ui -preset minimal
a2ui -preset dense
```

By default the local control socket lives at:

```text
$XDG_RUNTIME_DIR/a2ui/a2ui.sock
```

If `XDG_RUNTIME_DIR` is unavailable, A2UI falls back to a UID-scoped directory under `/tmp` rather than a global shared socket.

Only one interactive client is allowed in this checkpoint. Closing that client releases the interactive lease but does not destroy the daemon-owned session, Document, input values, focus, or table selection.

## The protocol is six operations

An agent does not send Bubble Tea code, Go source, ANSI escape sequences, or shell scripts. It describes UI state with six operations.

### `upsert`

Create a node or fully replace the props of an existing node.

```json
{"v":1,"seq":1,"op":"upsert","id":"status","type":"text","parent":"root","props":{"text":"Ready","variant":"title"}}
```

### `props`

Patch validated top-level properties on an existing node.

```json
{"v":1,"seq":2,"op":"props","id":"status","props":{"style":{"fg":"primary","bold":true}}}
```

### `text`

Append text to a `text` or `viewport` node without rebuilding the layout. This is the cheap path for streamed model output.

```json
{"v":1,"seq":3,"op":"text","id":"log","text":"compiled package daemon\n"}
```

### `remove`

Remove a node and its subtree.

```json
{"v":1,"seq":4,"op":"remove","id":"old-panel"}
```

### `focus`

Request semantic focus for a focusable input, selectable table, or scrollable viewport.

```json
{"v":1,"seq":5,"op":"focus","id":"command"}
```

### `commit`

Create a publication barrier. The acknowledgement means the committed semantic frame was actually rendered by the attached UI client, not merely accepted by the daemon.

```json
{"v":1,"seq":6,"op":"commit","frame":"ready"}
```

Those are the public mutation primitives. Renderer presets, terminal dimensions, caret position, viewport offset, animation frames, Unix socket control messages, and Linux integration are host concerns rather than extra agent commands.

## Why not generated Bash?

An LLM can generate a shell script that prints a beautiful interface. The problem is that a shell script is also executable authority.

A2UI deliberately keeps UI description on a data plane:

| Generated shell | A2UI operation stream |
|---|---|
| Can start arbitrary processes | Cannot start a process by describing a node |
| Can read/write arbitrary files with caller privileges | UI mutations only touch the bounded in-memory Document |
| Can make arbitrary network calls | The UI protocol has no network primitive |
| Can inject arbitrary terminal control sequences | Raw ANSI/terminal escape styling is not a protocol primitive |
| Can compose new shell commands at runtime | Public mutation vocabulary is fixed and strictly validated |
| Syntax success can still mean dangerous execution | Invalid A2UI mutations are rejected transactionally with zero Document change |
| Side effects are defined by generated code | Side effects cross only explicit host-controlled action boundaries |

This does **not** mean “the daemon has no privileges” or that every registered action is harmless. It means rendering an agent-generated interface does not require executing agent-generated code or creating a per-interface sandbox. Host actions remain capabilities and must be registered deliberately.

## The trust boundary

The authoritative split is small:

```text
agent
  │ six validated operations
  ▼
a2uid
  ├─ Session
  ├─ Engine
  ├─ Document
  ├─ Runtime projection
  ├─ Event broker
  └─ registered actions
       │
       │ secure local IPC
       ▼
a2ui
  ├─ Bubble Tea renderer
  ├─ caret
  ├─ viewport offset
  └─ animation
```

The daemon owns semantic state. The terminal client owns presentation mechanics.

The local Unix socket is an authority boundary because user interactions can change focus/input/selection and invoke pre-registered actions. Its parent directory is mode `0700` and the socket is mode `0600`.

A second daemon never blindly deletes a live socket. A stale socket is removed only after A2UI verifies that no daemon is listening on it.

## Closing the window is not closing the session

The persistent process model is the point:

```text
client A attaches
  ↓
user edits / selects
  ↓
client A closes
  ↓
a2uid keeps semantic state
  ↓
client B attaches
  ↓
current atomic snapshot is rendered
```

The following survive a client restart:

- Document and revision;
- focus;
- input values;
- selected table row;
- agent/session state;
- pending publication state.

These intentionally do not survive because they are local presentation state:

- input caret;
- viewport scroll offset and tail pin;
- animation phase;
- terminal dimensions.

## A commit means visible

Daemonization must not weaken `commit`.

Sending a snapshot over a Unix socket does not prove that the operator saw it. A2UI therefore uses a renderer acknowledgement:

```text
agent commit
  ↓
a2uid marks publication generation N pending
  ↓
snapshot N → a2ui
  ↓
Bubble Tea renders snapshot N
  ↓
frame_published(N) → a2uid
  ↓
Engine publishes N exactly once
  ↓
committed event → agent
```

A stale acknowledgement cannot publish a newer frame. A duplicate acknowledgement cannot create a second `committed`. If the terminal disappears before acknowledgement, publication remains pending and a later client can render and acknowledge the current generation.

## Efficient model output

NDJSON is verbose for initial structure, but A2UI does not require rebuilding the tree while content streams.

For long output, create the layout once and append with `text`:

```json
{"v":1,"seq":10,"op":"text","id":"answer","text":"next chunk"}
```

Few-shot examples in `assets/example-session.ndjson` and `assets/example-events.ndjson` help models reuse known-valid shapes.

Heavy reusable screen macros/templates are intentionally **not** part of this daemon checkpoint. They can be added later as a measured token-efficiency feature without mixing template expansion with process lifetime or IPC correctness.

## What A2UI does not give an agent

The public UI protocol does not provide:

- shell execution;
- arbitrary process spawning;
- arbitrary filesystem operations;
- arbitrary network requests;
- raw terminal escape execution;
- generated Bubble Tea commands;
- generated Go plugins;
- arbitrary host keymaps.

When the host exposes an action, that action is an explicit capability outside the six mutation operations and remains subject to the host's policy.

## Where it fits next

The persistent daemon is the right owner for later Omarchy integrations:

- system ANSI/Base16 theme mapping;
- Waybar state export;
- desktop notifications;
- Wayland clipboard;
- Vim-style navigation;
- systemd user service/socket activation.

Those integrations should attach to `a2uid`, not turn the Bubble Tea window into a second system daemon.
