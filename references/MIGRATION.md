# Migration from the original A2UI v1 sketch

The original archive remains useful as a design sketch, but several semantics changed during hardening. This file maps each review finding to the new owner.

| Original issue | Hardened owner / behavior |
|---|---|
| `props(box)` looked only in widget pool | `document.Document` owns node existence; containers receive props normally |
| `dirty/commit` had no publication effect | `runtime.State` tracks dirty vs published revision; commit produces `committed` barrier event |
| invalid props replaced widget with permanent ErrorComponent | failed candidate is discarded; ID remains recoverable |
| critical events silently dropped | bounded `runtime.EventBroker`; critical overflow is `runtime.backpressure_exceeded` |
| leaf could become parent | document invariant: only `box` and `viewport` are containers |
| action key depended on Go map iteration | tree-order binding index; duplicate key rejects candidate |
| nil registry/handler panic | registry validates handler and engine creates a safe empty registry |
| `MergeProps` mutated raw map before validation | candidate shallow merge then validate then commit |
| `input.force` stayed sticky | force extracted into one-shot `MutationPolicy`, never retained |
| table upsert retained old columns | normalized full replacement restores defaults |
| focus could point at removed/non-focusable node | runtime focus reconciliation after every relevant transition |
| border was omitted from width budget | `layout.ComputeBox` accounts for border + padding |
| row gap was ignored | gap semantics are direction-independent in `layout` |
| viewport height ignored terminal budget | `layout.ClampViewportHeight` |
| props schemas were disconnected | discriminated upsert union references every component props definition |
| runtime-only error codes absent from docs | namespaced error taxonomy in `PROTOCOL.md` |
| arbitrary invalid color silently ignored | invalid color rejects candidate; old state preserved |
| SKILL contradicted protocol | `PROTOCOL.md` explicitly normative; `SKILL.md` rewritten from it |
| scanner errors became clean EOF | bounded reader distinguishes clean EOF, malformed record, oversize and transport error |
| input could allocate unbounded tree/text | negotiated finite `protocol.Limits` enforced by reducer |
| C timeout implied cancellation | `external.Actor` marks itself DEGRADED after submitted call times out |
| reference implementation could not build | stdlib-only core + buildable `reference-impl` facade |
| NDJSON was protocol and transport at once | wire codec separated from session/semantic protocol |
| no remote/embedded transport model | HTTP streaming, MCP bridge, telemetry-only UDP profiles |


## Renderer V2 evolution

The first Bubble Tea adapter exposed another gap: it already attempted to render `viewport.Children`, while the v1 document law allowed only `box` to parent nodes. Renderer V2 resolves the contradiction by making `viewport` a real container while keeping all other node types leaves.

Presentation evolution is additive and strict: legacy documents remain valid, semantic variants/flex hints are optional, and renderer presets are deliberately outside the wire protocol. Animation state is renderer-local and cannot advance Document revision or fabricate `committed` events.


## Interactive Runtime V3 evolution

Renderer V2 still kept table selection in the Bubble Tea model and had no manual viewport offset or real caret. V3 promotes only semantic table selection into renderer-independent Runtime projection while keeping presentation mechanics local to the adapter.

Additional strict props are additive: selectable tables may provide `row_ids` and `action`; viewports may provide `scrollable`. Legacy tables without row IDs preserve/clamp numeric selection, and legacy non-scrollable viewports retain V2 clipping/follow-tail behavior.

Publication dirtiness is now separated from local redraw generation. Agent-driven mutations/focus continue to participate in the publication barrier, while local focus/input/table navigation, caret movement and viewport scrolling cannot fabricate `committed` acknowledgements. Bubble Tea rendering uses one atomic `Engine.PresentationSnapshot()` for coherent Document + Runtime state.


## Persistent daemon/client evolution

V3 still coupled semantic lifetime to the standalone terminal process. Epic 1 introduces `a2uid` as the persistent owner of Session + Engine and changes Bubble Tea to operate through a narrow semantic controller. `cmd/a2ui-runner` intentionally keeps the old in-process mode for development and compatibility.

Daemon/client synchronization uses a separate local `ipc` protocol rather than adding a recursive `upsert` or other public Agent operation. Reattach sends an atomic PresentationSnapshot; Document, focus, input values, table selection and pending publication survive terminal closure, while caret, viewport offset and animation state intentionally reset.

The standalone publication barrier becomes a remote generation ACK: writing a snapshot to the Unix socket is not enough to emit `committed`. Only the client that rendered the exact current generation can acknowledge it. A disconnect leaves the barrier pending for a later client.

Socket lifecycle is hardened for a tiling-desktop daemon: per-user private runtime path, strict modes, single-client lease, stale socket recovery under an advisory startup lock, and listener-owned unlink-on-close.
