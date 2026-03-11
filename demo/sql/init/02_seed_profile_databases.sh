#!/bin/sh
set -eu

for db in centian_poc_dev centian_poc_demo; do
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" \
    -f /docker-entrypoint-initdb.d/01_seed.sql
done

