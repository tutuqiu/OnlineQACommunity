#!/usr/bin/env bash
set -euo pipefail

# Step 1: switch to the backend project root.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

# Step 2: require .env so runtime config is explicit.
if [[ ! -f ".env" ]]; then
  echo "missing .env"
  echo "run: cp .env.example .env"
  echo "then edit DATABASE_URL in .env"
  exit 1
fi

# Step 3: load .env and export variables for child processes.
set -a
source ./.env
set +a

# Step 4: prepare runtime directory (binary, log, pid).
mkdir -p ./.run

# Step 5: sync dependencies and build the server binary.
go mod tidy
go build -o ./.run/onlineqa-backend ./cmd/server

# Step 6: avoid duplicate start when existing pid is alive.
if [[ -f ./.run/server.pid ]]; then
  pid="$(cat ./.run/server.pid || true)"
  if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
    echo "server already running pid=${pid}"
    exit 0
  fi
fi

# Step 7: start in background, persist pid, and print status.
nohup ./.run/onlineqa-backend > ./.run/server.log 2>&1 &
echo $! > ./.run/server.pid
echo "started pid=$(cat ./.run/server.pid)"
