# A2UI security evidence ledger

This file records what the current implementation actually proves. It is intentionally stricter than product positioning.

Status vocabulary:

- **VERIFIED** — implementation path and regression evidence support the bounded claim;
- **LIMITED** — claim is true only inside a stated boundary;
- **UNVERIFIED** — plausible or intended, but the required proof is missing;
- **OPEN** — an observed defect, flake, or prerequisite prevents a clean release claim.

## Evidence baseline

Authoritative Epic 1 `develop` baseline:

```text
540b8f6103d0c590c78a1e5e5f2ec93f7e7e7807
```

Documentation/agent-kit work is being prepared on:

```text
feature/developer-agent-kit
```

The detach/EOF lifecycle fix is commit:

```text
86482ed4c1be7c6b47dcce989c83619a55c8b46f
```

The first full branch verification including that fix and the shipped Omarchy fixture gate ran on:

```text
6ccbde7cf8ef2afa51a0fe257da6b3010cca97c3
GitHub Actions run 33991597869
Go 1.24.x
```

That run passed unit/conformance, race, vet, wire fuzz, document fuzz, and IPC fuzz. Before upstream submission, record the final reviewed SHA again after documentation-only edits and rerun the full gate.

## Claim matrix

| Claim | Status | Implementation evidence | Regression / check | Residual risk |
| --- | --- | --- | --- | --- |
| The public UI mutation vocabulary is fixed to six operations and has no shell/process/filesystem/network primitive. | **VERIFIED** | `protocol/types.go`, `references/PROTOCOL.md` | `go test ./protocol ./wire ./document` | Registered host actions are a separate capability boundary. |
| Unknown/invalid mutation data is rejected without partial authoritative Document mutation. | **VERIFIED** | `wire/json.go`, `document/reducer.go` | `TestFailedMutationRollsBackAuthoritativeState`, `TestNodeAndDepthLimitsAreTransactional`, `TestAggregateDocumentByteLimitIsTransactional` | Does not prove every possible CPU/connection DoS is bounded. |
| Duplicate JSON keys and unknown fields are rejected by strict JSON entry points. | **VERIFIED** | `wire/json.go` (`rejectDuplicateKeys`, `strictDecode`, `StrictUnmarshal`) | `go test ./wire`; wire fuzz smoke | Non-JSON transports still need equivalent boundary checks at their adapters. |
| Retained semantic state has finite negotiated/default resource limits. | **VERIFIED** | `protocol.DefaultLimits`, `document.Validate`, reducer accounting | `TestTextRetentionIsBounded`, `TestAggregateDocumentByteLimitIsTransactional`, table/node/depth tests | Runtime cost of valid content is not a complete DoS proof. |
| Local daemon/client IPC can carry a valid retained Document larger than the public 1 MiB agent record. | **VERIFIED** | `ipc/limits.go` | `TestIPCRecordLimitCoversRetainedDocumentBudget`, `TestClientCanAttachToSnapshotLargerThanAgentRecordLimit` | IPC budget remains finite; very large but valid snapshots still consume memory/CPU. |
| Default UDS directory/socket permissions restrict ordinary cross-user access. | **LIMITED** | `ipc/socket.go` | `go test ./ipc` socket mode/stale-socket tests | Same UID and root are not isolated. Unix mode bits are not app-level authentication. |
| A live daemon socket is not blindly removed during startup. | **VERIFIED** | `ipc.ListenUnix` active-probe + startup lock | socket startup/stress tests | Depends on normal Unix filesystem/socket semantics. |
| HTTP/MCP requests are authenticated. | **UNVERIFIED / CURRENTLY FALSE AS A GENERAL CLAIM** | `daemon/mcp.go`, `cmd/a2uid/main.go` show protocol validation but no authentication layer | code review; bind-address review | Current safe deployment guidance is loopback/dev only. Session ID and MCP headers are not credentials. |
| Agent-provided terminal text cannot execute ANSI/OSC terminal controls. | **VERIFIED** | `adapter/bubbletea/sanitize.go`, `render_helpers.go`, `render_components.go` | `TestSanitizeTextNeutralizesTerminalControls`, `TestRendererSanitizesInjectedDocumentNodes`, `TestSanitizeSplitSequenceAcrossAppends` | Strips ESC/CSI/OSC (incl. OSC 52 clipboard & OSC 8), C0/C1, Bidi overrides before styling and rendering. |
| Unknown host action IDs cannot execute a handler. | **VERIFIED** | `runtime/actions.go` registry lookup | action tests / `action.not_permitted` behavior | A registered handler is trusted host code and may have broad side effects. |
| Action timeout forcibly stops all handler work and rolls back effects. | **UNVERIFIED / NOT GUARANTEED** | `runtime/actions.go` uses context timeout | code review | Cancellation is cooperative; handler goroutine/side effects are not forcibly rolled back. |
| A stale renderer acknowledgement cannot publish a newer commit generation. | **VERIFIED** | `daemon/publication.go`, Engine publication-generation APIs, monotonic IPC snapshots | release-hardening publication tests, race suite | ACK proves renderer path, not human attention. |
| `Client.Close()` releases the single-client lease before a successful detach returns, without misreporting the expected post-ACK EOF as failure. | **VERIFIED** | server releases lease before `detach_ack`; `ipc.Client.requestMessage` gives an already-completed request response priority over `c.closed` | `TestClientCloseReleasesInteractiveLeaseBeforeReturning`; full unit + race in run `33991597869` | Unexpected transport loss before a response remains a real `ipc.connection_closed`. |
| Closing the renderer preserves daemon-owned semantic state. | **VERIFIED WITH CURRENT E2E COVERAGE** | daemon owns Engine/Session; IPC initial snapshot | reattach E2E + persistent-daemon tests | No disk persistence is promised across daemon restart/reboot. |
| `commit` means the human saw/read the frame. | **NOT A VALID CLAIM** | renderer sends exact-generation `frame_published` after its render path | publication tests | Terminal paint, window visibility, user attention, and comprehension are outside current proof. |

## Six-operation evidence

The public operations are defined in `protocol/types.go`:

```text
upsert
props
text
remove
focus
commit
```

A coherent hardened-session example is kept in:

```text
assets/examples/omarchy-choice.ndjson
```

and required by:

```bash
go test ./conformance -run TestOmarchyChoiceFixtureReplaysAllSixOperations
```

The shipped-example strict decoder also includes this fixture through `wire/examples_test.go`.

The conformance test proves that the fixture enters through the hardened envelope/session path, exercises every mutation type, removes its temporary node, leaves the expected focus, and creates a pending commit before publication.

It does **not** prove the final end-user CLI round trip because that CLI is not part of this documentation checkpoint.

## Transactional state evidence

Representative reducer tests:

```text
TestFailedMutationRollsBackAuthoritativeState
TestInvalidCreateDoesNotPoisonLaterRecovery
TestNodeAndDepthLimitsAreTransactional
TestAggregateDocumentByteLimitIsTransactional
TestRemoveFocusedConcernIsReportedAsEffectAndRootImmutable
```

Expected security interpretation:

```text
invalid mutation
    ↓
error
    ↓
authoritative Document unchanged
```

Do not turn this into the broader claim that arbitrary agent traffic cannot consume CPU or connections.

## Wire ambiguity evidence

`wire/json.go` rejects:

- duplicate object keys at any nesting depth;
- unknown struct fields;
- trailing/multiple JSON values;
- missing mandatory envelope fields;
- unsupported versions;
- envelope/payload sequence mismatch.

Verification commands:

```bash
go test ./wire
go test ./wire -run '^$' -fuzz '^FuzzDecodeRecord$' -fuzztime=3s
```

## Local IPC evidence

Relevant implementation:

```text
ipc/socket.go
ipc/client.go
ipc/limits.go
daemon/server.go
daemon/publication.go
```

Focused checks include:

```bash
go test ./ipc ./daemon
go test -race ./ipc ./daemon
```

The current branch inherits or adds tests for:

- first client / `ipc.client_busy`;
- lease release;
- stale socket recovery;
- exact publication generation;
- duplicate/stale ACK behavior;
- pending publication across reconnect;
- large retained snapshot attach;
- request-bound detach followed by immediate server EOF.

### Resolved lifecycle observation

Two independent PR runs reproduced the same intermittent symptom before the fix:

```text
run 33991148698
TestClientCloseReleasesInteractiveLeaseBeforeReturning
iteration 10
ipc.connection_closed: daemon connection closed
```

and later:

```text
run 33991464459
TestClientCloseReleasesInteractiveLeaseBeforeReturning
iteration 75
ipc.connection_closed: daemon connection closed
```

Root cause was client-side request completion ordering, not lease release ordering. The daemon correctly performed:

```text
release lease
→ write detach_ack
→ close connection
```

The client read loop could then decode the successful `detach_ack`, immediately observe the expected EOF, and close `c.closed`. Both the buffered successful request response and `c.closed` became ready; Go `select` was allowed to choose the closed branch and incorrectly return `ipc.connection_closed`.

Fix `86482ed4...` makes an already-completed request-bound response authoritative when `c.closed` is simultaneously ready. A genuine EOF without a completed response still yields `ipc.connection_closed`.

Full verification run `33991597869` on `6ccbde7...` passed both normal and race suites plus all fuzz smoke gates.

## HTTP access evidence

Current daemon HTTP path:

```text
cmd/a2uid -server <address>
    ↓
daemon.ServeHTTP
    ↓
MCP strict decode/header validation
    ↓
A2UI session/operation handling
```

No authentication middleware or credential check is present in the reviewed path.

Allowed current documentation claim:

> The MCP/HTTP bridge is suitable for controlled local/loopback development. Remote exposure requires an access-control design that this checkpoint does not provide.

Disallowed current claim:

> MCP headers or the A2UI session ID authenticate the caller.

## Terminal-control evidence

Reviewed path:

```text
agent text
  ↓
Document props / retained text
  ↓
renderText / table / input / action label paths
  ↓
wrapping / Lip Gloss
  ↓
terminal output
```

A dedicated A2UI escape sanitizer is implemented in `adapter/bubbletea/sanitize.go` and applied in `render_helpers.go` and `render_components.go`.

Verified regression matrix:

```text
ESC                             -> stripped
CSI cursor/control sequences    -> stripped (clear screen, cursor jumps, styling)
OSC 8 hyperlinks                -> stripped (visible text preserved, hidden URI removed)
OSC 52 clipboard                -> stripped (both BEL and ST terminators)
C1 controls (8-bit CSI/OSC)     -> stripped
CR / BS / C0 controls           -> CR converted to LF (avoids line overwrite), C0 dropped (except \n, \t)
split sequence across appends   -> stripped across propString + node Text boundaries
text props                      -> sanitized via propString
viewport text                   -> sanitized before wrapping and slicing
input value/placeholder         -> sanitized via SanitizeSingleLineText
table cells/titles              -> sanitized via SanitizeSingleLineText
action labels                   -> sanitized via SanitizeSingleLineText
```

Verified by `TestSanitizeTextNeutralizesTerminalControls`, `TestRendererSanitizesInjectedDocumentNodes`, and `TestSanitizeSplitSequenceAcrossAppends`.

## Host-action evidence

`runtime.ActionRegistry` provides:

- explicit registration by ID;
- unknown ID → `action.not_permitted`;
- bounded max inflight handlers;
- context timeout/cancellation;
- structured `action_result` / error events.

Residual risk:

```text
registered handler == trusted host capability
```

The timeout cancels the context; it does not guarantee forced goroutine termination or side-effect rollback.

## Publication evidence

The daemon validates exact generation before `Engine.PublishGeneration`.

Correct bounded claim:

> A stale or duplicate renderer publication acknowledgement cannot acknowledge a newer semantic publication generation.

Incorrect stronger claim:

> `committed` proves the user saw the screen.

## Required final verification before submission

On the final review SHA:

```bash
make smoke-http
make smoke-ipc
make test-reattach
make test-startup-stress
make test
make test-race
go vet ./...
go test ./conformance -run TestOmarchyChoiceFixtureReplaysAllSixOperations
go test ./wire -run '^$' -fuzz '^FuzzDecodeRecord$' -fuzztime=3s
go test ./document -run '^$' -fuzz '^FuzzApplyNeverBreaksDocumentInvariants$' -fuzztime=3s
go test ./ipc -run '^$' -fuzz '^FuzzDecodeMessage$' -fuzztime=3s
```

Record:

```text
final SHA
Go version
Bubble Tea/Lip Gloss versions
Omarchy version used for manual demo
terminal emulator
result of every gate
```

If any gate fails, update this evidence ledger instead of weakening the gate in prose.
