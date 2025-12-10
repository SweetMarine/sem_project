#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Running server on :8080..."
export PGHOST=localhost
export PGPORT=5432
export PGUSER=validator
export PGPASSWORD=val1dat0r
export PGDATABASE=project-sem-1

go run main.go &

SERVER_PID=$!
echo "[run.sh] Server started with PID $SERVER_PID"

sleep 3
