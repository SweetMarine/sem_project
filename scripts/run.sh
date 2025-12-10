#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Running server on :8080..."

go run main.go &

SERVER_PID=$!
echo "[run.sh] Server started with PID $SERVER_PID"

sleep 3
