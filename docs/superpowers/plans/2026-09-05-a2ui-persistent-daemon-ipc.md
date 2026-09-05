# A2UI Persistent Daemon IPC Implementation Plan

> **For agentic workers:** execute each task with RED → GREEN → refactor and verify the full matrix before release.

**Goal:** keep A2UI semantic/session state alive independently from the Bubble Tea terminal and reattach through one secure Unix-socket interactive client.

**Architecture:** `a2uid` owns Session + Engine; `a2ui` is a thin IPC-backed renderer/controller. Local IPC sends atomic presentation snapshots and semantic interactions; renderer-local caret/scroll never crosses the socket. Exact publication generations preserve the commit barrier across disconnect/reattach.

**Tech Stack:** Go 1.24, stdlib Unix sockets, existing Bubble Tea/Lip Gloss adapter.

**Spec:** `docs/superpowers/specs/2026-09-05-a2ui-persistent-daemon-ipc-design.md`

## Global constraints

- Public A2UI protocol operations remain unchanged.
- Exactly one interactive client.
- No disk persistence or system integration Epics in this branch.
- No Engine lock across socket writes.
- Snapshot/update queues bounded; all IPC records bounded and strict.
- Never publish merely because snapshot bytes were sent.

## Tasks

- [x] **IPC codec:** versioned strict messages, bounded NDJSON reader, serialized writer, oversize recovery.
- [x] **Unix socket lifecycle:** XDG/per-UID path, `0700/0600`, stale recovery, concurrent-start lock, listener-owned unlink.
- [x] **Single-client lease:** acquire after hello; busy rejection; EOF/detach release.
- [x] **Initial snapshot:** atomic Document + Runtime + publication metadata.
- [x] **Controller boundary:** local Engine controller retained; IPC controller added without fallback Engine.
- [x] **Remote interactions:** focus, input set/submit, table move/activate, action key.
- [x] **Publication ACK:** exact generation, duplicate idempotence, stale protection, disconnect/reattach pending barrier.
- [x] **Daemon Agent path:** existing Session sequencing + Engine mutation, snapshot notification without fake publish.
- [x] **Bounded slow-client behavior:** one coalescing update slot; write deadline; Engine lock released before write.
- [x] **Entrypoints:** `cmd/a2uid` and `cmd/a2ui`; standalone runner remains.
- [x] **Persistent E2E:** client A semantic state → detach → agent reorder/commit → client B restore/ACK.
- [x] **Regression hardening:** recoverable diagnostics vs disconnect, handshake/lease race, concurrent stale-socket startup TOCTOU.
- [x] **Docs:** README, SKILL, IMPLEMENTATION, TRANSPORTS, MIGRATION.
- [ ] **Exact environment gate:** run unmodified Go 1.24 module with upstream Bubble Tea/Lip Gloss when network/toolchain is available.

## Release verification

Run on the exact release tree:

```bash
go test ./... -count=1
go vet ./...
go test -race ./... -count=1
go test ./wire -run '^$' -fuzz '^FuzzDecodeRecord$' -fuzztime=3s
go test ./document -run '^$' -fuzz '^FuzzApplyNeverBreaksDocumentInvariants$' -fuzztime=3s
go test ./ipc -run '^$' -fuzz '^FuzzDecodeMessage$' -fuzztime=3s
```

If Go 1.24/upstream dependencies cannot be loaded, preserve `go.mod` and report `IMPLEMENTED / ENVIRONMENT GATE PENDING`; do not downgrade the project.
