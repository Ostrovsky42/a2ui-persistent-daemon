#!/usr/bin/env bash
set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$project_root"

PORT=${PORT:-18090}
SESSION=${SESSION:-omarchy-choice}
TMP_DIR=$(mktemp -d "${TMPDIR:-/tmp}/a2ui-roundtrip.XXXXXX")
chmod 700 "$TMP_DIR"
SOCKET="$TMP_DIR/a2ui.sock"
DAEMON_LOG="$TMP_DIR/a2uid.log"
DAEMON_PID=""

cleanup() {
  status=$?
  if [[ -n "$DAEMON_PID" ]]; then
    kill "$DAEMON_PID" 2>/dev/null || true
    wait "$DAEMON_PID" 2>/dev/null || true
  fi
  if [[ $status -ne 0 && -f "$DAEMON_LOG" ]]; then
    printf 'roundtrip demo: daemon log on failure:\n' >&2
    cat "$DAEMON_LOG" >&2
  fi
  rm -rf "$TMP_DIR"
  exit "$status"
}
trap cleanup EXIT INT TERM

printf '==> [1/6] Building a2ui and a2uid binaries...\n'
go build -o "$TMP_DIR/a2uid" ./cmd/a2uid
go build -o "$TMP_DIR/a2ui" ./cmd/a2ui

printf '==> [2/6] Launching persistent daemon on %s (HTTP :%s)...\n' "$SOCKET" "$PORT"
"$TMP_DIR/a2uid" -socket "$SOCKET" -server "127.0.0.1:$PORT" -session "$SESSION" >"$DAEMON_LOG" 2>&1 &
DAEMON_PID=$!

# Wait for daemon readiness
for _ in $(seq 1 50); do
  if [[ -S "$SOCKET" ]] && curl --silent --output /dev/null "http://127.0.0.1:$PORT/status"; then
    break
  fi
  sleep 0.1
done

printf '==> [3/6] Querying daemon status with `a2ui status`...\n'
"$TMP_DIR/a2ui" status -server "http://127.0.0.1:$PORT"

printf '==> [4/6] Agent publishes Omarchy choice screen with `a2ui send`...\n'
"$TMP_DIR/a2ui" send -server "http://127.0.0.1:$PORT" -session "$SESSION" assets/examples/omarchy-choice.ndjson -verbose

# Verify status shows published nodes
"$TMP_DIR/a2ui" status -server "http://127.0.0.1:$PORT"

printf '==> [5/6] Simulating user interaction and testing `a2ui wait-event`...\n'
# In background, simulate user interaction via a2ui interact
(
  sleep 0.5
  "$TMP_DIR/a2ui" interact -socket "$SOCKET" -input 'comment=Deploying to staging' -submit comment
) &

EVENT=$("$TMP_DIR/a2ui" wait-event -server "http://127.0.0.1:$PORT" -session "$SESSION" -timeout 5s)
printf '    Captured event: %s\n' "$EVENT"

if [[ "$EVENT" != *"ev=submit"* || "$EVENT" != *"id=comment"* ]]; then
  printf 'roundtrip demo: unexpected event output: %s\n' "$EVENT" >&2
  exit 1
fi

printf '==> [6/6] Agent updates panel with success confirmation...\n'
cat <<JSON | "$TMP_DIR/a2ui" send -server "http://127.0.0.1:$PORT" -session "$SESSION" -
{"op":"text","id":"choice-title","text":"Choice confirmed: continuing workflow..."}
{"op":"commit","frame":"choice-confirmed"}
JSON

"$TMP_DIR/a2ui" status -server "http://127.0.0.1:$PORT"

printf '\n✅ Round-trip demo PASS: Agent published UI -> User interacted -> Event delivered to Agent -> UI updated\n'
