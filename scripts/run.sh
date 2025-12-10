#!/usr/bin/env bash
set -euo pipefail

echo "[run.sh] Running server on :8080..."

go run ./cmd/server
