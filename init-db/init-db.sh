#!/bin/bash

set -e


psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" <<-EOSQL
    CREATE DATABASE botbot;
    GRANT ALL PRIVILEGES ON DATABASE botbot TO postgres;
EOSQL


psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" botbot -c 'CREATE EXTENSION hstore;'
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" botbot < /pg-tmp/schema.sql
