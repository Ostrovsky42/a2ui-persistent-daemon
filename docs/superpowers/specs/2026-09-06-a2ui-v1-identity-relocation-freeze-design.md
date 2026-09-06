# A2UI V1 Identity-Preserving Relocation Freeze — Design

## Goal

Close the final pre-freeze semantic gap in A2UI Protocol V1 without adding a seventh mutation, new node types, catalogs, fallbacks, derived table views, or new UI features.

The checkpoint has exactly two concerns:

1. existing-node `upsert` can relocate/reorder a node while preserving identity;
2. `hello_ack.components` is frozen as the endpoint capability contract for V1 node types.

## Authoritative baseline

The checkpoint starts from `develop` after merge of Adaptive Data Surfaces R1. No earlier feature SHA is authoritative once `develop` advances.

## Hard boundary

This checkpoint MUST NOT:

- change `protocol.Version`;
- add a mutation type;
- add a node type;
- add fallback/catalog negotiation;
- add sorting/filtering/search;
- add approval-gate execution;
- make the daemon a capability broker;
- move renderer-local state into Document;
- weaken strict unknown-type or unknown-property rejection.

## Existing-node `upsert` relocation semantics

`upsert` remains the full-props-replacement operation.

For an existing ID:

- omitted `parent` means keep the current parent;
- explicit `parent` equal to the current parent keeps that parent;
- explicit `parent` naming another container atomically relocates the existing node/subtree;
- omitted `index` with unchanged parent preserves sibling order;
- omitted `index` with a changed parent appends to the target parent's children;
- explicit `index` reorders the node within the final target parent's child order;
- an index outside the valid insertion range appends, matching creation semantics;
- the node ID and descendant IDs are not removed/recreated by relocation;
- a same-type relocation MUST preserve renderer-independent runtime identity associated with the surviving ID;
- a simultaneous type change continues to use the existing type-change rule: runtime state for that node is recreated as required by the new type.

The move is part of the same transactional candidate as props/type changes. Any failure leaves the authoritative Document unchanged.

## Structural rejection rules

Relocation MUST reject:

- moving `root` (existing root immutability rule);
- missing target parent with `document.parent_not_found`;
- leaf target parent with `document.parent_not_container`;
- moving a node under itself;
- moving a node under one of its descendants;
- any resulting depth/child/resource/invariant violation.

Self/descendant moves use `document.cycle` as the semantic error. Because the candidate is rejected before becoming authoritative, the current Document remains valid and unchanged.

## Runtime identity invariant

For same-type existing-node relocation, surviving identity is keyed by node ID, not tree position.

Therefore relocation alone MUST NOT clear or recreate:

- `InputValues[id]`;
- semantic `FocusedID` when the node remains focusable;
- `TableSelections[id]`;
- descendant runtime state for a moved subtree.

Tree-order-derived behavior such as focus traversal and action binding traversal MAY change because the authoritative tree order changed. The currently focused surviving ID itself remains focused unless another independent semantic rule makes it non-focusable.

Adapter-local state remains adapter-owned and keyed/reconciled by the existing adapter rules; this checkpoint does not introduce a protocol guarantee for carrying adapter-local state across detach/new attachment.

## `hello_ack.components` capability contract

For A2UI V1, `hello_ack.components` is normative endpoint capability advertisement.

A conforming endpoint MUST advertise the complete set of V1 node types that it guarantees it can accept and present through its conforming renderer path for that session.

An agent MUST treat the advertised set as an allowlist for node generation in that session. If the agent knows a newer component that is absent from the set, it MUST degrade before sending by expressing the UI with advertised V1 components.

Receiving an unknown/unadvertised V1 node type remains a strict schema error. V1 does not add a generic node-level fallback field or catalog subsystem.

The current daemon/Bubble Tea endpoint advertises the seven V1 node types returned by `protocol.AllNodeTypes()`.

## TDD evidence

Required RED coverage before production change:

1. same-parent existing-ID reorder fails under the current reducer;
2. cross-parent existing-ID relocation fails with `document.reparent_unsupported` under the current reducer;
3. runtime input value/focus survives relocation only after relocation becomes legal;
4. stable table selection survives relocation only after relocation becomes legal;
5. self/descendant relocation is rejected transactionally.

Capability advertisement is a characterization/freeze contract over already-present behavior; it need not manufacture a fake RED if current code already returns exactly `protocol.AllNodeTypes()`.

## Definition of Done

- existing-node same-parent reorder works transactionally;
- existing-node cross-parent relocation works transactionally;
- subtree identity is preserved;
- cycle/missing-parent/non-container/resource rejection is deterministic and rollback-safe;
- runtime input/focus/table selection preservation is covered;
- `hello_ack.components` exact advertisement contract is covered;
- normative protocol docs and practical protocol guide match implementation;
- no schema/node/op/version expansion;
- full current-head CI is green: formatting, `go test ./...`, race, vet, and all configured fuzz smoke jobs;
- PR is Ready only after current-head CI is green.
