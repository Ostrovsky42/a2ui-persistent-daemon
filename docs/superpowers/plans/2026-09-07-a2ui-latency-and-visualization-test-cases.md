# A2UI: latency baseline and visualization test cases

Date: 2026-09-07. Status: exploratory baseline and repeatable live corpus; not a release-performance claim.

This document complements [the live feedback plan](2026-09-07-p0-live-feedback-and-ux-followup.md).

## What “prompt to UI” means

```text
user sends prompt → Codex reasoning/tool selection → a2ui_publish → daemon → Bubble Tea frame → frame ACK
```

The repository currently observes only the right half. Record separately:

| Marker | Meaning | Capture |
| --- | --- | --- |
| T0 | user submits prompt | manual stopwatch or host trace |
| T1 | Codex starts `a2ui_publish` | harness trace; unavailable to daemon |
| T2 | `a2ui_publish` returns | MCP tool timing |
| T3 | matching `committed` returns | `a2ui_wait_event`; includes harness overhead |
| T4 | frame is useful to a person | manual observation; ACK does not prove attention |

Report T0→T4 as experience, not renderer time. Compare T1→T2 and T1→T3 separately.

## Live exploratory baseline

Environment: local Linux daemon, attached headless Bubble Tea client, `a2ui-mcp` connected through the current Codex session. No terminal-paint profiler, video capture, or host prompt timestamp was available.

Five two-operation samples (`upsert text`, `commit`) produced:

| Metric | Values, ms | Median |
| --- | --- | --- |
| T1→T2: publish returns | 8, 7, 9, 17, 15 | 9 |
| T1→T3: `committed` through MCP | 687, 601, 550, 501, 404 | 550 |

Operation-count probe:

| operations in one publish | T1→T2, ms | T1→T3, ms |
| --- | --- | --- |
| 2 | 12 | 359 |
| 8 | 11 | 239 |
| 20 | 16 | 161 |

The falling T1→T3 values show harness scheduling dominates this probe. It does not justify transport batching, although the current client does send operations one at a time. Revisit batching only after in-process timestamps show T1→T2 grows with operation count.

## Run protocol

1. Start a fresh daemon and attach one real terminal client.
2. Record commit, Go/OS/terminal versions, preset, terminal size and session.
3. Clear events, warm up once, then run five samples per case at 80 and 120 columns.
4. Record T0–T4, operation count, node count, rows, payload bytes and `pending_publish`.
5. Report p50/p95; do not mix human pause with machine latency.
6. With tester consent, capture a terminal recording to assess paint, focus and readability.

## Prompt corpus

All prompts use current A2UI V1 nodes. Charts, local live filtering and unregistered actions are outside P0 until implemented.

### P0-VIS-01 — deployment browser and filter

```text
Use A2UI. Show Russian “Развёртывания” with a search input and selectable dense table.
Use these rows: api-gateway/production/healthy, billing-worker/production/healthy,
catalog-sync/staging/deploying, checkout-web/production/guarded, cron-cleanup/development/idle,
data-export/staging/healthy, event-router/production/healthy, feature-flags/staging/paused,
image-worker/production/healthy, job-scheduler/development/idle, knowledge-index/staging/deploying,
ledger-api/production/guarded, metrics-collector/production/healthy, notification-hub/staging/healthy,
order-processor/production/healthy, payment-reconciler/staging/paused, queue-consumer/development/idle,
report-builder/production/healthy. Focus search. On Enter, wait for real submit, filter case-insensitively
by service, environment and state, then update the table. Show “Найдено: N из 18”, keyboard help and stable row IDs.
```

Acceptance: 18 rows initially; `worker` gives 2; empty query restores 18; no match has an explicit empty state.

### P0-VIS-02 — incident triage

```text
Use A2UI to show “Инцидент INC-4821”: muted summary, remediation progress 0.65,
and a selectable table: payments-api / critical / 18 min / Mira; checkout-web / high / 12 min / Jon;
inventory-sync / medium / 5 min / Oleg. Ask me to select a service, wait for real select,
then update the screen. Do not claim the incident is resolved.
```

Acceptance: readable at 80 columns; `progress.value` is 0..1; user text has no implementation ID.

### P0-VIS-03 — release approval

```text
Use A2UI to present a release gate for 2.18.0. Show a compact table with
“Продолжить в staging”, “Отложить релиз”, “Открыть changelog”. Use stable row IDs and an explicit action.
Explain selection records intent only and starts no deployment. Wait for one real select,
show the choice in Russian, then stop accepting this gate's further selections.
```

Acceptance: only the first valid selection advances; queued later selections do not repeat it.

### P0-VIS-04 — queue health

```text
Use A2UI to show “Очереди обработки” as dense selectable table: Queue, Depth, Oldest job, Rate/min, Status.
Create 30 rows queue-01 through queue-30: depth 0..12000, oldest job 0s..42m, rate 0..900;
queues above 8000 say “Требует внимания”. Focus the table and tell me arrows and Home/End navigate it.
Selection returns its queue ID.
```

Acceptance: 120-column view retains columns; 80-column fallback is comprehensible; selection stays coherent through Home/End.

### P0-VIS-05 — migration checklist

```text
Use A2UI to show “Миграция tenant-42”: Snapshot created — complete; Schema validated — complete;
Backfill running — 42%; Verification pending — waiting. Use semantic progress only for Backfill.
Add a selectable table: “Посмотреть журнал”, “Повторить проверку”, “Остановить после текущего batch”.
Wait for a real selection and show the chosen next action without executing it.
```

Acceptance: only the running step has progress; result explicitly says no external action ran.

### P0-VIS-06 — log investigation

```text
Use A2UI to show “Проверка ошибок worker”. Add a scrollable viewport with 60 short timestamped log lines,
including five ERROR lines with worker and retry count. Under it add a selectable table of the five errors.
Focus the viewport first and explain PageUp/PageDown/Home/End. Selecting an error returns its stable ID
and updates a short summary.
```

Acceptance: viewport scrolling creates no fake commits; table is reachable by Tab; retained text stays within limits.

## Next optimization decision

If T1→T2 rises with operation count, evaluate batching while retaining contiguous sequence and partial-failure semantics. If T1→T3 is high while T1→T2 stays low, instrument daemon receipt, renderer View and ACK before changing transport. If T0→T1 dominates, renderer changes cannot improve prompt-to-UI time.
