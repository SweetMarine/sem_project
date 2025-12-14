#!/usr/bin/env bash
set -e

psql "postgres://validator:val1dat0r@localhost:5432/project-sem-1?sslmode=disable" <<SQL
CREATE TABLE IF NOT EXISTS prices (
	id SERIAL PRIMARY KEY
);
SQL
