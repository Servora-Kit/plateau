#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage:
  setup-integration-env.sh [options]

Waits for OpenFGA readiness, initializes a dedicated integration-test store via
manifests/scripts/openfga/openfga.sh using a temporary env file (never touching
the repository's .env), and emits the environment variables required by Go
integration tests.

Options:
  --api-url URL          OpenFGA API URL (default: FGA_API_URL or http://127.0.0.1:${OPENFGA_HTTP_PORT:-18080})
  --model FILE           OpenFGA model file (default: manifests/openfga/fga.mod)
  --store-name NAME      OpenFGA test store name (default: plateau_integration_test)
  --wait-timeout SECS    Seconds to wait for OpenFGA /healthz (default: 60)
  --github-env           Append KEY=VALUE lines to $GITHUB_ENV instead of printing shell exports
  -h, --help             Show this help
EOF
}

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd -- "${script_dir}/../../.." && pwd)"
openfga_script="${repo_root}/manifests/scripts/openfga/openfga.sh"

api_url="${FGA_API_URL:-}"
model_file="${repo_root}/manifests/openfga/fga.mod"
store_name="plateau_integration_test"
wait_timeout=60
write_github_env=0

while (($# > 0)); do
  case "$1" in
    --api-url)
      [[ $# -ge 2 ]] || { echo "--api-url requires a value" >&2; exit 2; }
      api_url="$2"
      shift 2
      ;;
    --model)
      [[ $# -ge 2 ]] || { echo "--model requires a value" >&2; exit 2; }
      model_file="$2"
      shift 2
      ;;
    --store-name)
      [[ $# -ge 2 ]] || { echo "--store-name requires a value" >&2; exit 2; }
      store_name="$2"
      shift 2
      ;;
    --wait-timeout)
      [[ $# -ge 2 ]] || { echo "--wait-timeout requires a value" >&2; exit 2; }
      wait_timeout="$2"
      shift 2
      ;;
    --github-env)
      write_github_env=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

for tool in curl fga jq awk; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "required command not found: $tool" >&2
    exit 127
  }
done

[[ -f "$openfga_script" ]] || { echo "openfga script not found: $openfga_script" >&2; exit 1; }
[[ -f "$model_file" ]] || { echo "model file not found: $model_file" >&2; exit 1; }

if [[ -z "$api_url" ]]; then
  api_url="http://127.0.0.1:${OPENFGA_HTTP_PORT:-18080}"
fi
api_url="${api_url%/}"

health_url="${api_url}/healthz"
deadline=$((SECONDS + wait_timeout))
until curl -fsS "$health_url" >/dev/null 2>&1; do
  if ((SECONDS >= deadline)); then
    echo "timed out after ${wait_timeout}s waiting for OpenFGA at ${health_url}" >&2
    exit 1
  fi
  sleep 1
done

tmp_env="$(mktemp "${TMPDIR:-/tmp}/plateau-openfga-env.XXXXXX")"
trap 'rm -f "$tmp_env"' EXIT

bash "$openfga_script" init \
  --api-url "$api_url" \
  --model "$model_file" \
  --store-name "$store_name" \
  --env-file "$tmp_env" >&2

read_env_key() {
  local key="$1"
  awk -v key="$key" '
    index($0, key "=") == 1 {
      print substr($0, length(key) + 2)
      exit
    }
  ' "$tmp_env"
}

store_id="$(read_env_key FGA_STORE_ID)"
model_id="$(read_env_key FGA_MODEL_ID)"

[[ -n "$store_id" ]] || { echo "failed to read FGA_STORE_ID from ${tmp_env}" >&2; exit 1; }
[[ -n "$model_id" ]] || { echo "failed to read FGA_MODEL_ID from ${tmp_env}" >&2; exit 1; }

pg_user="${POSTGRES_USER:-postgres}"
pg_password="${POSTGRES_PASSWORD:-postgres}"
pg_port="${POSTGRES_PORT:-5432}"
iam_db="${IAM_DATABASE_NAME:-plateau_iam}"
admin_db="${ADMIN_DATABASE_NAME:-plateau_admin}"

iam_dsn="${IAM_TEST_POSTGRES_DSN:-postgres://${pg_user}:${pg_password}@127.0.0.1:${pg_port}/${iam_db}?sslmode=disable}"
admin_dsn="${ADMIN_TEST_POSTGRES_DSN:-postgres://${pg_user}:${pg_password}@127.0.0.1:${pg_port}/${admin_db}?sslmode=disable}"

emit_var() {
  local key="$1" value="$2"
  if ((write_github_env == 1)); then
    [[ -n "${GITHUB_ENV:-}" ]] || { echo "GITHUB_ENV is not set" >&2; exit 1; }
    printf '%s=%s\n' "$key" "$value" >>"$GITHUB_ENV"
  else
    printf 'export %s=%q\n' "$key" "$value"
  fi
}

emit_var "IAM_TEST_POSTGRES_DSN" "$iam_dsn"
emit_var "ADMIN_TEST_POSTGRES_DSN" "$admin_dsn"
emit_var "FGA_API_URL" "$api_url"
emit_var "IAM_TEST_FGA_URL" "$api_url"
emit_var "IAM_TEST_FGA_STORE_ID" "$store_id"
emit_var "IAM_TEST_FGA_MODEL_ID" "$model_id"

if ((write_github_env == 1)); then
  printf 'Exported integration test env to GITHUB_ENV (FGA_API_URL=%s, store=%s, model=%s)\n' \
    "$api_url" "$store_id" "$model_id" >&2
fi
