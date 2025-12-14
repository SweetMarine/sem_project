#!/usr/bin/env bash
set -e

echo "[run.sh] Starting server on :8080..."

go run main.go &
SERVER_PID=$!

# Ждём, пока сервер реально начнёт слушать порт
for i in {1..30}; do
  if curl -s http://localhost:8080 >/dev/null; then
    echo "[run.sh] Server is up"
    break
  fi
  echo "[run.sh] Waiting for server..."
  sleep 1
done
