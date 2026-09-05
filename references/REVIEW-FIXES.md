# Initial review → hardened implementation traceability

This table records how the concrete defects found in the original sketch were closed. It is intended to prevent future refactors from reintroducing the same class of bug under a different implementation.

| Original problem | Hardened invariant / implementation | Primary proof |
|---|---|---|
| `props(box)` failed because containers were absent from widget pool | Document owns node existence; runtime projection is secondary | document reducer tests |
| `dirty` / `commit` did not control actual publication | Renderer-controlled projection generation + `Engine.Publish()` barrier | engine publication tests |
| Invalid props could permanently replace a widget with `ErrorComponent` | Invalid candidate never replaces authoritative state; later valid upsert can reuse ID | invalid-create recovery test |
| Critical events silently dropped through `select default` | Bounded reliable `EventBroker`; saturation returns `runtime.backpressure_exceeded` | runtime broker tests |
| Leaf nodes could become parents | Only `box` and `viewport` may own children | document invariant tests |
| Action key winner depended on Go map iteration | Deterministic tree-order binding index; conflict rejects candidate | binding conflict tests |
| Nil registry/handler could panic | Engine installs safe registry; registration rejects nil handlers | action tests |
| `MergeProps` mutated raw state before validation | Candidate merge → normalize/validate → commit | transactional props tests |
| `input.force` became sticky | `force` extracted as one-shot effect and never retained in Document props | runtime input tests |
| Table upsert behaved partly like merge | `upsert` always normalizes a full replacement from defaults | reducer props tests |
| Focus could remain stale after removal/type/selectability change | Focus reconciled deterministically against candidate tree | runtime focus tests |
| Layout props existed but were not honored coherently | Renderer-neutral box model explicitly accounts for border/padding/gaps/viewport clamp | layout tests |
| Props schemas existed but were not wired into operations | Draft 2020-12 discriminated operation/envelope schemas | schema contract tests + example validation |
| Runtime emitted codes absent from documentation | Stable error taxonomy centralized and documented | protocol tests/docs |
| `invalid_color` was documented but silently ignored | Invalid style token rejects candidate with `schema.invalid_color` | protocol props tests |
| SKILL and protocol behavior conflicted | `PROTOCOL.md` is normative; `SKILL.md` explicitly subordinate | documentation contract |
| NDJSON scanner errors looked like EOF | Clean EOF, malformed record, oversize record and transport read failure are distinct | wire tests |
| Unbounded trees/text/props could exhaust memory | Negotiated finite limits + aggregate `max_document_bytes` | resource tests |
| CGO/native worker lacked lifecycle and timeout truthfulness | Thread-affine actor with health states; timeout/panic degrades actor | external actor tests |
| Action timeout ignored semaphore wait | Deadline covers inflight-slot wait and handler execution | action saturation test |
| Handler panic could terminate process | Panic recovered to `action.failed` | action panic test |
| Native executor panic could terminate process | Panic recovered; actor transitions to `DEGRADED` | external panic test |
| No-op mutations fabricated Document revisions | Revision advances only on authoritative state change | no-op revision test |
| Focus/input local changes were invisible to publication | Separate runtime projection generation | engine/runtime publication tests |
| Outbound code could serialize invalid envelopes | Wire encoder self-validates; MCP/UDP reuse common validation | wire/MCP/UDP tests |
| HTTP handler could return `200` before discovering invalid response | First response validated before committing success status | HTTP invalid-outbound test |
| Event envelope `seq:0` disappeared due `omitempty` | Envelope sequence is explicitly serialized | HTTP mid-stream round-trip test |
| Multiple pending commits could exceed publishable event capacity | Commit admission bounded by event capacity | engine commit admission test |
| UDP accepted weaker payload shapes than common wire | UDP delegates semantic envelope validation to common wire validator | UDP telemetry payload test |
| Duplicate JSON keys could create differential parsing | Duplicate keys rejected recursively at all JSON entry points | wire/MCP/UDP tests |
