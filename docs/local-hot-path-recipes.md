# A2UI V1 local hot-path recipes

These recipes use only the frozen A2UI V1 vocabulary. They are guidance for agents that publish semantic data once and let the terminal runtime own exploration. Navigation, focus movement, caret movement, table inspection, responsive layout and viewport scrolling are local presentation/runtime work; only committed intent crosses the agent boundary.

Do not put navigation instructions such as "use arrows" into the Document. The terminal renderer derives contextual help from the focused component and declared semantic actions.

## 1. choice

Use a title plus a selectable table with stable `row_ids`. Optional explanatory text may describe the decision, not the mechanics of operating the terminal.

```jsonl
{"v":1,"seq":1,"op":"upsert","id":"choice-title","type":"text","parent":"root","props":{"text":"Choose a worker","variant":"title"}}
{"v":1,"seq":2,"op":"upsert","id":"workers","type":"table","parent":"root","props":{"selectable":true,"action":"worker.choose","columns":[{"title":"Worker","width":18},{"title":"State","width":12}],"rows":[["worker-a","ready"],["worker-b","busy"],["worker-c","idle"]],"row_ids":["worker-a","worker-b","worker-c"]}}
{"v":1,"seq":3,"op":"focus","id":"workers"}
{"v":1,"seq":4,"op":"commit","frame":"worker-choice"}
```

Row movement remains local. Explicit table activation is the semantic boundary and produces one `select` event containing the selected `row_id`.

## 2. form

Use ordinary input nodes. The runtime owns transient input value and caret state while the human edits. Do not republish `value` on each keystroke; use `force:true` only for an intentional one-shot overwrite.

```jsonl
{"v":1,"seq":1,"op":"upsert","id":"form-title","type":"text","parent":"root","props":{"text":"Deployment note","variant":"title"}}
{"v":1,"seq":2,"op":"upsert","id":"note","type":"input","parent":"root","props":{"placeholder":"Reason for deployment","action":"deployment.note.submit"}}
{"v":1,"seq":3,"op":"focus","id":"note"}
{"v":1,"seq":4,"op":"commit","frame":"deployment-note"}
```

Typing, caret movement, Home/End, Backspace and Delete are local. Enter is the semantic boundary and produces one `submit` event with the current value.

## 3. master-detail

Publish all detail fields already known to the agent in the selectable table. Stable `row_ids` are mandatory for agent-updated data. At widths where the adaptive table uses master-detail presentation, the renderer projects the selected row into the detail pane locally.

```jsonl
{"v":1,"seq":1,"op":"upsert","id":"services","type":"table","parent":"root","props":{"selectable":true,"action":"service.open","columns":[{"title":"Service","width":18},{"title":"State","width":12},{"title":"Owner","width":16},{"title":"Task","width":28},{"title":"Age","width":8}],"rows":[["api","ready","team-a","Serving traffic","12s"],["worker","busy","team-b","Rebuilding index","3m"],["mailer","idle","team-a","No queued mail","8s"]],"row_ids":["service-api","service-worker","service-mailer"]}}
{"v":1,"seq":2,"op":"focus","id":"services"}
{"v":1,"seq":3,"op":"commit","frame":"services"}
```

Selection movement immediately changes the locally projected details and does not require `a2ui_wait_event` or a new publish. Only explicit activation returns `select` to the agent.

## 4. status-dashboard

Compose the dashboard from existing `box`, `text`, `table`, `progress` and `actions` nodes. Use row layout plus `responsive:"stack"` when the same semantic content should switch from side-by-side to vertical presentation on narrow terminals. Do not invent a dashboard node type.

```jsonl
{"v":1,"seq":1,"op":"upsert","id":"dashboard","type":"box","parent":"root","props":{"dir":"col","gap":1}}
{"v":1,"seq":2,"op":"upsert","id":"summary-row","type":"box","parent":"dashboard","props":{"dir":"row","responsive":"stack","gap":1}}
{"v":1,"seq":3,"op":"upsert","id":"queue-card","type":"box","parent":"summary-row","props":{"variant":"card","flex":{"grow":1,"basis":20,"min_width":16}}}
{"v":1,"seq":4,"op":"upsert","id":"queue-text","type":"text","parent":"queue-card","props":{"text":"Queue: 18","variant":"subtitle"}}
{"v":1,"seq":5,"op":"upsert","id":"sync-card","type":"box","parent":"summary-row","props":{"variant":"card","flex":{"grow":1,"basis":20,"min_width":16}}}
{"v":1,"seq":6,"op":"upsert","id":"sync-progress","type":"progress","parent":"sync-card","props":{"value":0.72,"label":"Sync","state":"normal"}}
{"v":1,"seq":7,"op":"upsert","id":"dashboard-actions","type":"actions","parent":"dashboard","props":{"variant":"toolbar","items":[{"key":"r","label":"Retry","action":"dashboard.retry"}]}}
{"v":1,"seq":8,"op":"commit","frame":"status-dashboard"}
```

Terminal resize is presentation-only. It must not create semantic events or require a new generation solely because the width changed.

## 5. progress-log

Use `progress` for semantic progress and a scrollable `viewport` for log history. `follow_tail:true` may keep the live edge visible until the human scrolls away; the viewport offset remains adapter-local.

```jsonl
{"v":1,"seq":1,"op":"upsert","id":"job-progress","type":"progress","parent":"root","props":{"value":0.45,"label":"Indexing","state":"loading"}}
{"v":1,"seq":2,"op":"upsert","id":"job-log","type":"viewport","parent":"root","props":{"scrollable":true,"follow_tail":true}}
{"v":1,"seq":3,"op":"upsert","id":"job-log-text","type":"text","parent":"job-log","props":{"text":"phase 1 complete\nphase 2 started","variant":"code"}}
{"v":1,"seq":4,"op":"focus","id":"job-log"}
{"v":1,"seq":5,"op":"commit","frame":"job-progress"}
```

The agent may independently publish later progress/log data updates. Human scrolling through existing history stays local and emits no semantic event.
