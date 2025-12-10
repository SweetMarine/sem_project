#!/usr/bin/env bash
set -euo pipefail

echo "[prepare.sh] Downloading Go dependencies..."
go mod tidy

echo "[prepare.sh] Preparing PostgreSQL schema..."

psql "postgres://validator:val1dat0r@localhost:5432/project-sem-1?sslmode=disable" <<'SQL'
CREATE TABLE IF NOT EXISTS prices (
    id          SERIAL PRIMARY KEY,
    product_id  INTEGER       NOT NULL,
    created_at  DATE          NOT NULL,
    name        TEXT          NOT NULL,
    category    TEXT          NOT NULL,
    price       NUMERIC(10,2) NOT NULL
);
SQL

echo "[prepare.sh] Done."
