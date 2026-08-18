#!/bin/sh
set -eu

: "${POSTGRES_SERVICE_DATABASES:?POSTGRES_SERVICE_DATABASES is required}"

for database in $(printf '%s' "$POSTGRES_SERVICE_DATABASES" | tr ',' ' '); do
  case "$database" in
    ''|*[!a-zA-Z0-9_]* )
      echo "invalid database name: $database" >&2
      exit 1
      ;;
  esac

  psql \
    --set=database_name="$database" \
    --set=ON_ERROR_STOP=1 \
    --dbname="${PGDATABASE:-postgres}" <<'SQL'
SELECT format('CREATE DATABASE %I', :'database_name')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_database WHERE datname = :'database_name'
)\gexec
SQL

done
