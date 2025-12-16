#!/usr/bin/env bash
set -euo pipefail

HOST="${POSTGRES_HOST:-localhost}"
PORT="${POSTGRES_PORT:-5432}"
DB="${POSTGRES_DB:-project-sem-1}"
USER="${POSTGRES_USER:-validator}"
PASSWORD="${POSTGRES_PASSWORD:-val1dat0r}"

export PGPASSWORD="$PASSWORD"

psql -h "$HOST" -p "$PORT" -U "$USER" -d "$DB" <<'SQL'
CREATE TABLE IF NOT EXISTS prices (
  id          INTEGER PRIMARY KEY,
  name        VARCHAR(255) NOT NULL,
  category    VARCHAR(255) NOT NULL,
  price       NUMERIC(10,2) NOT NULL,
  create_date DATE NOT NULL
);
SQL
