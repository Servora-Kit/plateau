#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  cat <<'EOF'
Usage:
  openfga.sh init [options]
  openfga.sh apply [options]

Options:
  --api-url URL       OpenFGA API URL (default: FGA_API_URL or http://localhost:18080)
  --model FILE        Authorization model file
  --store-name NAME   Store name used by init (default: plateau)
  --store-id ID       Existing store ID used by apply/init
  --env-file FILE     dotenv output/input file (default: .env)
  -h, --help          Show this help
EOF
}

command_name="${1:-}"
if [[ -z "$command_name" || "$command_name" == "-h" || "$command_name" == "--help" ]]; then
  usage
  [[ -n "$command_name" ]] && exit 0 || exit 2
fi
shift

api_url=""
model_file="manifests/openfga/fga.mod"
store_name="${OPENFGA_STORE_NAME:-plateau}"
store_id=""
env_file=".env"

while (($# > 0)); do
  case "$1" in
    --api-url) [[ $# -ge 2 ]] || { echo "--api-url requires a value" >&2; exit 2; }; api_url="$2"; shift 2 ;;
    --model) [[ $# -ge 2 ]] || { echo "--model requires a value" >&2; exit 2; }; model_file="$2"; shift 2 ;;
    --store-name) [[ $# -ge 2 ]] || { echo "--store-name requires a value" >&2; exit 2; }; store_name="$2"; shift 2 ;;
    --store-id) [[ $# -ge 2 ]] || { echo "--store-id requires a value" >&2; exit 2; }; store_id="$2"; shift 2 ;;
    --env-file) [[ $# -ge 2 ]] || { echo "--env-file requires a value" >&2; exit 2; }; env_file="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

for tool in fga jq awk; do
  command -v "$tool" >/dev/null 2>&1 || {
    echo "required command not found: $tool" >&2
    exit 127
  }
done

[[ -f "$model_file" ]] || { echo "model file not found: $model_file" >&2; exit 1; }

# Read only dotenv key/value pairs; never source an arbitrary environment file.
dotenv_get() {
  local key="$1"
  [[ -f "$env_file" ]] || return 0
  awk -v key="$key" '
    /^[[:space:]]*(export[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*=/ {
      line=$0
      sub(/^[[:space:]]*/, "", line)
      sub(/^export[[:space:]]+/, "", line)
      name=line
      sub(/=.*/, "", name)
      if (name == key) {
        value=line
        sub(/^[^=]*=/, "", value)
        sub(/[[:space:]]+#.*$/, "", value)
        gsub(/^"|"$/, "", value)
        print value
        exit
      }
    }
  ' "$env_file"
}

config_value() {
  local key="$1" value
  value="${!key-}"
  if [[ -n "$value" ]]; then
    printf '%s' "$value"
    return
  fi
  dotenv_get "$key"
}

if [[ -z "$api_url" ]]; then
  api_url="$(config_value FGA_API_URL)"
  [[ -n "$api_url" ]] || api_url="http://localhost:18080"
fi

atomic_upsert_env() {
  local dir tmp updates_text
  dir="$(dirname "$env_file")"
  mkdir -p "$dir"
  [[ -f "$env_file" ]] || : >"$env_file"
  tmp="$(mktemp "${env_file}.tmp.XXXXXX")"
  updates_text="$(printf '%s\034' "$@")"
  if ! awk -v updates="$updates_text" '
    BEGIN {
      count = split(updates, entries, "\034")
      for (i = 1; i <= count; i++) {
        separator = index(entries[i], "=")
        if (separator == 0) continue
        key = substr(entries[i], 1, separator - 1)
        values[key] = substr(entries[i], separator + 1)
        order[i] = key
      }
    }
    {
      line = $0
      probe = line
      sub(/^[[:space:]]*(export[[:space:]]+)?/, "", probe)
      name = probe
      sub(/=.*/, "", name)
      if (name in values) {
        if (!(name in seen)) print name "=" values[name]
        seen[name] = 1
        next
      }
      print line
    }
    END {
      for (i = 1; i <= count; i++) {
        key = order[i]
        if (key != "" && !(key in seen)) print key "=" values[key]
      }
    }
  ' "$env_file" >"$tmp"; then
    rm -f "$tmp"
    return 1
  fi
  mv "$tmp" "$env_file" || {
    rm -f "$tmp"
    return 1
  }
}

write_env() {
  local model_id="$1"
  atomic_upsert_env "FGA_API_URL=$api_url" "FGA_STORE_ID=$store_id" "FGA_MODEL_ID=$model_id"
}

fga_args=(--api-url "$api_url")
fga_call() { fga "${fga_args[@]}" "$@"; }


write_model() {
  local response model_id
  response="$(fga_call model write --store-id "$store_id" --file "$model_file")" || {
    echo "failed to write OpenFGA authorization model" >&2
    exit 1
  }
  model_id="$(jq -er '.authorization_model.id // .authorization_model_id // .id // empty' <<<"$response" 2>/dev/null)" || {
    echo "OpenFGA model write returned no model ID" >&2
    exit 1
  }
  printf '%s' "$model_id"
}

case "$command_name" in
  init)
    if [[ -z "$store_id" ]]; then
      stores="$(fga_call store list)" || {
        echo "failed to list OpenFGA stores" >&2
        exit 1
      }
      matching_stores="$(jq -cer --arg name "$store_name" '[.stores[]? | select(.name == $name)]' <<<"$stores" 2>/dev/null)" || {
        echo "OpenFGA store list returned invalid JSON" >&2
        exit 1
      }
      store_count="$(jq -r 'length' <<<"$matching_stores")"
      if ((store_count > 1)); then
        echo "multiple OpenFGA stores found with exact name: $store_name" >&2
        exit 1
      fi
      if ((store_count == 1)); then
        store_id="$(jq -er '.[0].id // empty' <<<"$matching_stores" 2>/dev/null)" || {
          echo "matching OpenFGA store returned no store ID" >&2
          exit 1
        }
      fi
    fi
    if [[ -z "$store_id" ]]; then
      response="$(fga_call store create --name "$store_name")" || {
        echo "failed to create OpenFGA store" >&2
        exit 1
      }
      store_id="$(jq -er '.store.id // .id // empty' <<<"$response" 2>/dev/null)" || {
        echo "OpenFGA store create returned no store ID" >&2
        exit 1
      }
    fi
    model_id="$(write_model)"
    write_env "$model_id"
    printf 'OpenFGA store ready: %s\nOpenFGA model ready: %s\n' "$store_id" "$model_id"
    ;;
  apply)
    [[ -n "$store_id" ]] || store_id="$(config_value FGA_STORE_ID)"
    [[ -n "$store_id" ]] || { echo "store ID required: use --store-id or FGA_STORE_ID" >&2; exit 2; }
    model_id="$(write_model)"
    atomic_upsert_env "FGA_MODEL_ID=$model_id"
    printf 'OpenFGA model applied: %s\n' "$model_id"
    ;;
  *)
    echo "unknown command: $command_name" >&2
    usage >&2
    exit 2
    ;;
esac
