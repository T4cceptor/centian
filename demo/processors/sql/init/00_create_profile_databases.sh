#!/bin/sh
set -eu

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<'EOSQL'
CREATE DATABASE centian_poc_dev;
CREATE DATABASE centian_poc_demo;
EOSQL

