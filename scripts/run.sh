#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Setting PostgreSQL env..."
export PGUSER=validator
export PGPASSWORD=val1dat0r
export PGHOST=localhost
export PGPORT=5432
export PGDATABASE=project-sem-1

echo "[run.sh] Starting server on :8080..."

go run main.go 2>&1 | tee server.log &

sleep 3
