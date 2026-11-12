#!/bin/sh
set -eu

: "${POSTGRES_SERVICE_DATABASES:?POSTGRES_SERVICE_DATABASES is required}"
: "${IAM_DATABASE_NAME:=plateau_iam}"
: "${IAM_DATABASE_ROLE:=plateau_iam_app}"
: "${IAM_DATABASE_PASSWORD:?IAM_DATABASE_PASSWORD is required}"

validate_identifier() {
  case "$1" in
  '' | *[!a-zA-Z0-9_]*)
    echo "invalid PostgreSQL identifier: $1" >&2
    exit 1
    ;;
  esac
}

validate_identifier "$IAM_DATABASE_NAME"
validate_identifier "$IAM_DATABASE_ROLE"

psql \
  --set=role_name="$IAM_DATABASE_ROLE" \
  --set=role_password="$IAM_DATABASE_PASSWORD" \
  --set=ON_ERROR_STOP=1 \
  --dbname="${PGDATABASE:-postgres}" <<'SQL'
SELECT format('CREATE ROLE %I LOGIN PASSWORD %L', :'role_name', :'role_password')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_roles WHERE rolname = :'role_name'
)
\gexec
SQL

for database in $(printf '%s' "$POSTGRES_SERVICE_DATABASES" | tr ',' ' '); do
  validate_identifier "$database"

  if [ "$database" = "$IAM_DATABASE_NAME" ]; then
    psql \
      --set=database_name="$database" \
      --set=database_owner="$IAM_DATABASE_ROLE" \
      --set=ON_ERROR_STOP=1 \
      --dbname="${PGDATABASE:-postgres}" <<'SQL'
SELECT format('CREATE DATABASE %I OWNER %I', :'database_name', :'database_owner')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_database WHERE datname = :'database_name'
)
\gexec

SELECT format('ALTER DATABASE %I OWNER TO %I', datname, :'database_owner')
FROM pg_database
WHERE datname = :'database_name'
  AND datdba <> (SELECT oid FROM pg_roles WHERE rolname = :'database_owner')
\gexec
SQL
  else
    psql \
      --set=database_name="$database" \
      --set=ON_ERROR_STOP=1 \
      --dbname="${PGDATABASE:-postgres}" <<'SQL'
SELECT format('CREATE DATABASE %I', :'database_name')
WHERE NOT EXISTS (
  SELECT 1 FROM pg_database WHERE datname = :'database_name'
)
\gexec
SQL
  fi
done
