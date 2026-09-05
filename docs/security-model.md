# A2UI security model

A2UI is designed to reduce the authority needed to **display and interact with an agent-generated UI**. It is not a sandbox for an entire AI agent and it is not a general isolation boundary for a compromised user account.

The useful comparison is narrow:

> For the task of presenting a dynamic terminal UI, six validated A2UI mutations expose less authority than executing arbitrary model-generated Bash with the same user privileges and no additional confinement.

That is the security claim this project should defend.

## Scope

This document covers:

- public A2UI mutations and strict decoding;
- retained Document/runtime state;
- persistent `a2uid` ownership;
- local daemon/client Unix-socket IPC;
- Bubble Tea terminal rendering;
- registered host actions;
- the optional MCP/HTTP bridge.

It does not claim to secure:

- a fully compromised user account;
- a malicious/root terminal emulator;
- arbitrary registered host action handlers;
- arbitrary third-party agent tools outside A2UI;
- remote HTTP exposure without an explicit authentication layer;
- human attention or comprehension of a rendered frame.

## Assets

Security-sensitive assets include:

- files and processes available to the user account;
- terminal and clipboard state;
- input values entered by the user;
- integrity of the semantic UI/session;
- registered host-action capabilities;
- CPU, memory, file descriptors, and connection capacity;
- correctness of publication and user-event ordering.

## Actors

### Untrusted or semi-trusted agent

The model or agent supplies protocol records and UI text. Treat all of it as untrusted input.

### Other Unix user

Should not be able to mutate a user's daemon through the default local socket permissions.

### Same-UID process

Is **not** isolated by `0700` directory and `0600` socket permissions. Another process running as the same user may have equivalent filesystem/socket access.

### Network caller

Relevant only when the optional HTTP bridge is enabled. Current MCP/HTTP headers and A2UI session identifiers are protocol metadata, not authentication credentials.

### Trusted daemon/runtime

`a2uid`, the semantic Engine, reducer, session layer, event broker, and registered action handlers are part of the trusted computing base.

### Trusted renderer and terminal

The Bubble Tea adapter and terminal emulator are also in the TCB for what appears on screen. A renderer acknowledgement proves a renderer-path event, not that a human read or understood the frame.

## Data flow

```text
agent data
    ↓
transport access policy
    ↓
strict decode + session sequencing
    ↓
candidate validation + finite retained limits
    ↓
Document + Runtime projection
    ↓
atomic presentation snapshot
    ↓
trusted renderer
    ↓
terminal

user input
    ↓
renderer interaction
    ↓
daemon Runtime/Event Broker
    ↓
agent event transport

explicit action ID
    ↓
registered host handler
    ↓
host side effect
```

## Boundary 1: public UI mutations

The public mutation vocabulary is fixed:

```text
upsert
props
text
remove
focus
commit
```

Those operations describe and synchronize UI state. They contain no primitive equivalent to:

- `exec` / shell command execution;
- arbitrary process spawn;
- arbitrary filesystem read/write;
- arbitrary network request;
- dynamic Go/plugin loading.

This is the basis for the "zero-sandbox UI plane" positioning: A2UI does not need to execute a generated UI program merely to render the UI.

It does **not** mean the daemon process has zero privileges.

## Boundary 2: strict decoding and transactional mutation

Strict JSON paths reject malformed input, duplicate object keys, unknown struct fields, unsupported versions, and sequence mismatches.

State mutation follows a candidate model:

```text
current Document
    + mutation
    → candidate
    → validation
    → structural/resource checks
    → authoritative replacement
```

If validation fails, the current Document must remain unchanged. This reduces parser ambiguity and partial-state corruption, but does not by itself prevent every CPU or connection DoS.

## Boundary 3: retained resource limits

Default limits are finite. They bound agent record size, total retained Document size, node/depth/children counts, retained text, tables, pending events, pending mutations, action concurrency, and telemetry datagrams.

The local daemon/client snapshot budget is separate from the public agent-record budget because a valid retained Document can be larger than one inbound mutation record.

These are **retained-state bounds**, not a complete denial-of-service proof. Connection churn, expensive renderer behavior, or pathological but valid workloads still need independent operational limits and measurement.

## Boundary 4: Unix domain socket

The default control path is:

```text
$XDG_RUNTIME_DIR/a2ui/a2ui.sock
```

with UID-scoped `/tmp` fallback.

The runtime directory is forced to mode `0700`; socket and startup lock use `0600`. Startup probes an existing socket and does not blindly unlink a live daemon.

This prevents ordinary cross-user access through filesystem permissions under normal Unix assumptions.

Residual risk:

- same-UID processes are not isolated;
- root is not isolated;
- filesystem namespace or host compromise invalidates this boundary;
- socket mode is not an application-level authentication protocol.

## Boundary 5: HTTP/MCP bridge

The optional HTTP bridge currently validates MCP/A2UI protocol structure, not caller identity.

The daemon accepts a configurable `-server` address. The current handler does not implement authentication or authorization, and an A2UI `session` ID must not be treated as a secret credential.

Deployment policy for the current version:

```text
bind the HTTP bridge to loopback for development only
```

Do not document remote/LAN exposure as supported until an explicit access-control design and regression tests exist.

## Boundary 6: terminal text and escape sequences

The public protocol has semantic style tokens rather than an ANSI styling operation. That is useful, but it does **not** prove that arbitrary control bytes embedded inside ordinary text are safe.

The current Bubble Tea renderer passes agent-supplied text into wrapping/styling paths without a dedicated A2UI terminal-control sanitizer proven by regression tests.

Therefore the following claim is currently **not established**:

> "Agent text cannot inject terminal control sequences."

Before making that claim, test at least:

- ESC / CSI sequences;
- OSC 8 hyperlinks;
- OSC 52 clipboard sequences;
- C1 controls;
- carriage return and backspace behavior;
- escape sequences split across incremental `text` appends;
- text-bearing table/input/action labels and other props.

Tests should inspect renderer bytes in a controlled environment. Do not send security probes directly to a user's working terminal.

## Boundary 7: registered host actions

`actions` nodes carry semantic action IDs and JSON args. They do not contain executable source code.

The host's `ActionRegistry` decides which IDs are registered. An unknown action returns `action.not_permitted`.

However, a registered handler runs with host-process privileges. It is a real capability boundary.

Current timeout semantics cancel a context; they do not forcibly terminate an arbitrary handler goroutine or undo side effects already performed.

Hosts should therefore:

- register a small allowlist;
- validate action args inside the handler/policy layer;
- avoid generic "run command" actions;
- require confirmation for destructive capabilities where appropriate;
- design handlers to respect context cancellation;
- treat handler implementation as trusted code.

## Boundary 8: publication acknowledgement

`commit` is a publication barrier.

In daemon mode, the daemon only publishes the current pending generation when the renderer returns the exact matching `frame_published(generation)` token. Duplicate acknowledgements are idempotent and stale generations cannot acknowledge newer state.

This protects protocol ordering against stale renderer acknowledgements.

It does not prove:

- the terminal emulator successfully painted every pixel/cell;
- the window was visible to the user;
- the user was looking at the window;
- the user understood the content.

Public documentation should say "entered the renderer publication path" rather than "the human saw it" unless stronger evidence is added.

## UI spoofing and social engineering

A valid A2UI document can still display misleading text. Declarative UI safety does not solve phishing, fake confirmation language, or confusing labels.

Hosts should visually distinguish trusted host chrome from agent-controlled content if A2UI is later used for high-risk approvals.

## User input privacy

Input values live in daemon Runtime projection and can be emitted to the agent through semantic user events such as submit.

Do not use an ordinary A2UI input for secrets unless the protocol/host explicitly defines a secret-handling policy. Current input semantics are not a password-vault boundary.

## Dependencies and compromised host

The security model depends on trusted Go runtime behavior, Bubble Tea/Lip Gloss, terminal emulator behavior, and the host environment.

A compromised dependency or already-compromised user account is outside the protection offered by the six-operation mutation vocabulary.

## Recommended current deployment

For the current checkpoint:

```text
Agent protocol ingress: local/trusted harness or loopback MCP bridge
Daemon IPC:            default UID-local Unix socket
Interactive clients:   one
Host actions:          explicit allowlist only
Disk persistence:      none promised
Remote HTTP:           unsupported until access control exists
Terminal controls:     treat agent text as untrusted; sanitization proof pending
```

## Security work still open

Before an upstream security pitch stronger than the bounded comparison above:

1. add and test terminal-control sanitization/escaping policy;
2. define HTTP access control or formally keep the bridge loopback-only;
3. decide policy for sensitive input values;
4. document/measure CPU and connection-level DoS limits;
5. review action argument validation and confirmation policy;
6. define trusted-versus-agent-controlled UI chrome for high-risk prompts;
7. reproduce and close any lifecycle/race flakiness before release evidence is marked fully green.

See [`security-evidence.md`](security-evidence.md) for the current evidence ledger.
