# P0.3 local acknowledgement acceptance

This is a real Bubble Tea check of adapter-local feedback. It uses the frozen
V1 fixture in `docs/manual/p0-3-local-hot-path.ndjson`; the acknowledgement
text is renderer chrome, not fixture content and not an A2UI mutation.

Start a fresh daemon and attach one terminal client, then load the fixture:

```bash
./bin/a2ui send -server http://127.0.0.1:8080 -session p03-ack \
  docs/manual/p0-3-local-hot-path.ndjson
./bin/a2ui -socket /the/daemon/a2ui.sock -preset minimal
```

Use a second terminal to consume the initial `committed` event. For every
boundary below, inspect the frame before sending any further A2UI operations.

1. Focus `p03-services`, move to any row, then press `Enter` once. The frame
   immediately contains `Selected` and `Waiting for agent…`. Exactly one
   `select` event is available; another event read times out.
2. Press `Tab` to focus `p03-note`, type a note, then press `Enter` once. The
   frame immediately contains `Submitted`, the submitted value, and `Waiting
   for agent…`. Exactly one `submit` event is available.
3. With a non-input focus, press `r` once. The frame immediately contains
   `Action accepted` and `Waiting for agent…`; exactly the existing one action
   dispatch event is available. A stock daemon may report the existing
   `action.not_permitted` result for an unregistered `service.retry`; that is
   unrelated to local dispatch acknowledgement and must not create a second
   event.
4. Publish and commit any ordinary V1 update. The local acknowledgement and
   waiting text disappear on that next accepted publication.

For steps 1–4, compare `a2ui status` before and after the local key press:
Document revision and publication generation must not change until the final
agent publication. The acknowledgement must never be sent as agent-authored
text and it must not appear in the document operations.
