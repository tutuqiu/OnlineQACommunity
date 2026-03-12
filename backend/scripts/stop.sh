#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

if [[ ! -f ./.run/server.pid ]]; then
  echo "no server pid file"
  exit 0
fi

pid="$(cat ./.run/server.pid)"
if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
  kill "${pid}"
  echo "stopped pid=${pid}"
else
  echo "process already stopped"
fi

rm -f ./.run/server.pid
