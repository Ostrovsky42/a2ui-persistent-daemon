#!/usr/bin/env bash
set -euo pipefail

project_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
PORT=18080 "$project_root/scripts/dev/a2ui" smoke-http
