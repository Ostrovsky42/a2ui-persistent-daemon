# AIR R0 renderer hardware result

Date: 2026-09-08

Branch: `experiment/20260908-air-r0-web-renderer`

Tested exact HEAD: `0af28eba1b0fbb138f3ad9ebddebea9ba373959f`

Verdict: **PASS**

This record pins the real-machine renderer-lifetime proof to the exact commit that was actually tested. It does not claim that later commits on the experiment branch were exercised on hardware.

## Confirmed end-to-end behavior

- TUI accepted a human submit with `value = from-tui`.
- Closing only the TUI preserved the daemon and session.
- A detached agent publication changed the semantic Document to the Web phase.
- The local Web renderer displayed the new publication from the same daemon-owned state.
- Web interaction produced a semantic submit event with the Unicode value `from-веб`.
- Starting a second interactive renderer while Web held the lease was rejected with `ipc.client_busy`.
- Closing Web released the interactive lease.
- TUI reattached without restarting the daemon.
- Reattached TUI displayed the Web-phase Document and the runtime value written through Web.
- Daemon PID remained `2786281` throughout the end-to-end renderer switch.
- Final status reported `has_client=true` and `pending_publish=false` while the final TUI remained attached.

## Architectural conclusion

The real-machine sequence passed:

```text
TUI
  -> human runtime mutation
  -> detach
  -> agent semantic Document mutation while detached
  -> Web
  -> semantic human response
  -> detach
  -> TUI
```

with one daemon/session and one interactive writer at a time.

This is hardware evidence for the R0 architectural claim that renderer lifetime is independent from daemon-owned semantic/runtime state. The Unicode value `from-веб` additionally exercised UTF-8 input through the Web interaction path.

## Evidence boundary

After this tested commit, the experiment branch received renderer-local presentation fixes for issues observed during the hardware run (table default column width, input allocated width, and a duplicated percentage in the smoke fixture). Those later changes do not invalidate the architectural PASS above, but they require a short visual smoke on the later candidate before that later commit can inherit a hardware-ready status.

The full acceptance procedure remains in `docs/audits/AIR_R0_RENDERER_HARDWARE_ACCEPTANCE_2026-09-08.md`.
