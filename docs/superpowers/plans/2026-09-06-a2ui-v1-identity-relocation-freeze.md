# A2UI V1 Identity-Preserving Relocation Freeze Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the final V1 tree-mutation semantic gap by making existing-node `upsert` relocate/reorder identity-preservingly and freeze `hello_ack.components` as the endpoint capability contract.

**Architecture:** Keep the six-operation wire surface unchanged. Extend only the existing `document.applyUpsert` candidate mutation so topology changes happen atomically before invariant validation; rely on existing Runtime reconciliation to preserve state for surviving same-type IDs. Freeze the already-present component advertisement behavior with characterization/conformance tests and normative docs rather than adding catalogs or fallback nodes.

**Tech Stack:** Go, existing `protocol`/`document`/`runtime`/`engine` packages, GitHub Actions CI.

**Spec:** `docs/superpowers/specs/2026-09-06-a2ui-v1-identity-relocation-freeze-design.md`

## Global Constraints

- `protocol.Version` remains `1`.
- Public mutation vocabulary remains exactly `upsert`, `props`, `text`, `remove`, `focus`, `commit`.
- Node catalog remains exactly `box`, `text`, `viewport`, `table`, `input`, `actions`, `progress`.
- Strict unknown type/property rejection remains unchanged.
- No fallback/catalog subsystem, filtering/search, approval execution, or capability broker is added.
- Failed relocation must leave the authoritative Document unchanged.

---

### Task 1: Reducer relocation semantics

**Files:**
- Modify: `document/reducer_test.go`
- Modify: `document/reducer.go`

**Interfaces:**
- Consumes: existing `protocol.Operation{Op: OpUpsert, ID, Type, Parent, Index, Props}`.
- Produces: existing-node topology update with unchanged public API.

- [ ] **Step 1: Write failing reducer tests**

Add tests that construct real Documents and assert:

```go
func intPtr(n int) *int { return &n }
```

```go
func TestExistingUpsertReordersWithinParent(t *testing.T) {
    // root -> box -> [a,b,c]
    // upsert existing b with Index=0
    // expect box.Children == []string{"b","a","c"}
    // expect b.Parent unchanged, Effect.Changed true,
    // Effect.Removed empty, TypeChanged false.
}
```

```go
func TestExistingUpsertRelocatesSubtreeAcrossParents(t *testing.T) {
    // root -> left,right; left -> panel -> input
    // upsert existing panel with Parent="right", Index=0
    // expect left no longer references panel, right references panel,
    // panel.Parent == "right", child.Parent == "panel", no removals.
}
```

```go
func TestExistingUpsertRejectsCycleTransactionally(t *testing.T) {
    // parent -> child, then attempt parent.Parent="child"
    // expect document.cycle and original Document deep-equal to before.
}
```

Also pin missing target parent and non-container target parent for existing-node relocation.

- [ ] **Step 2: Run CI and verify RED**

Push tests without reducer production changes and require the current-head `ci` run to fail in unit tests because current code returns `document.reparent_unsupported` / ignores existing-node `Index`.

- [ ] **Step 3: Implement minimal topology helpers**

In `document/reducer.go`, add focused unexported helpers:

```go
func removeChild(children []string, id string) []string
func insertChild(children []string, id string, index *int) []string
func wouldCreateCycle(d Document, id, targetParent string) bool
```

`insertChild` uses existing creation semantics: explicit in-range index inserts there; otherwise append.

Update existing-node `applyUpsert` flow:

```go
targetParent := n.Parent
if op.Parent != "" {
    targetParent = op.Parent
}
parentChanged := targetParent != n.Parent
reorderRequested := op.Index != nil
```

Validate target parent existence/container when parent changes. Reject self/descendant target with `document.cycle`. If topology changes, remove ID from old parent children, insert into target parent children, update `n.Parent`, and mark `eff.Changed`.

Do not touch `eff.Removed`; do not synthesize type change.

- [ ] **Step 4: Verify GREEN**

Require current-head CI unit tests to pass and no existing reducer tests regress.

- [ ] **Step 5: Commit**

Commit reducer tests and implementation as one RED→GREEN implementation slice after RED evidence is recorded in PR history/body.

---

### Task 2: Runtime identity preservation

**Files:**
- Modify: `engine/engine_test.go`

**Interfaces:**
- Consumes: Task 1 existing-node relocation behavior.
- Produces: regression proof that runtime state remains keyed by surviving same-type ID.

- [ ] **Step 1: Add engine characterization tests**

Add one test for input/focus:

```go
// create left/right boxes + input under left
// set runtime input to "typed" and focus input
// relocate same input ID to right using OpUpsert
// assert InputValue(id) == "typed" and FocusedID() == id
```

Add one test for table selection:

```go
// create selectable table with stable row_ids under left
// select second row
// relocate same table ID to right
// assert selection index and RowID are unchanged
```

No production runtime change is expected; if these fail after Task 1, treat that as a real ownership regression and fix only the minimal reconciliation defect.

- [ ] **Step 2: Run focused/full CI**

Require all unit/conformance/race/vet checks green.

- [ ] **Step 3: Commit**

Commit runtime identity tests separately.

---

### Task 3: Freeze component capability contract

**Files:**
- Modify: `session/session_test.go`
- Modify: `references/PROTOCOL.md`
- Modify: `docs/protocol-guide.md`

**Interfaces:**
- Consumes: existing `protocol.AllNodeTypes()` and `Session.Negotiate`.
- Produces: normative meaning for `HelloAck.Components` without wire expansion.

- [ ] **Step 1: Strengthen characterization test**

Replace the count-only assertion with exact set/order equality:

```go
want := protocol.AllNodeTypes()
if !reflect.DeepEqual(ack.Components, want) {
    t.Fatalf("components=%v want=%v", ack.Components, want)
}
```

Keep version/limits/state assertions.

- [ ] **Step 2: Update normative protocol**

In `references/PROTOCOL.md` Session negotiation, state that:

- `hello_ack.components` is the complete node-type allowlist guaranteed by that endpoint/session;
- agents MUST NOT send components absent from the set;
- newer agents MUST degrade before sending using advertised V1 types;
- unknown received node types remain strict errors;
- V1 defines no generic fallback/catalog mechanism.

In `upsert`, replace `document.reparent_unsupported` semantics with the exact existing-node relocation/reorder rules from the spec.

- [ ] **Step 3: Update practical guide**

Mirror the same existing-node semantics and error list in `docs/protocol-guide.md`.

- [ ] **Step 4: Verify docs/tests**

Require current-head CI green.

- [ ] **Step 5: Commit**

Commit capability freeze docs/test.

---

### Task 4: Final freeze verification

**Files:**
- Update PR description only; no mutable SHA/run pin in prose.

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces: merge-ready V1 freeze checkpoint.

- [ ] **Step 1: Review changed files**

Confirm no `assets/schema.json`, node catalog, mutation enum, `protocol.Version`, action capability, filtering/search, or renderer feature expansion occurred.

- [ ] **Step 2: Verify current-head CI**

Require the GitHub `ci` workflow attached to the current PR HEAD to complete successfully for:

```text
format gate
go test ./...
go test -race ./...
go vet ./...
wire fuzz smoke
document reducer fuzz smoke
IPC codec fuzz smoke
```

- [ ] **Step 3: Mark Ready only after GREEN**

Keep PR Draft while current-head checks are absent/pending/red. Mark Ready only after exact current head is green and mergeable.

- [ ] **Step 4: Merge with head protection**

Merge only using `expected_head_sha` equal to the freshly re-read PR head.

- [ ] **Step 5: Confirm authoritative develop**

Fetch `develop` after merge and record the resulting merge commit as the A2UI V1 pre-freeze closure baseline.
