#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Starting server on :8080..."
go run main.go &
SERVER_PID=$!

# ждем up сервера
for i in {1..30}; do
  if curl -s http://localhost:8080 >/dev/null; then
    echo "[run.sh] Server is up"
    exit 0
  fi
  echo "[run.sh] Waiting for server..."
  sleep 1
done

echo "[run.sh] Server did not start in time"
kill "$SERVER_PID" || true
exit 1