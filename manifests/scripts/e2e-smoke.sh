#!/usr/bin/env bash
set -euo pipefail

# Platform service e2e smoke test.
# Assumes the dev compose stack is running (`just compose-up`).
#
# Usage:
#   ./scripts/e2e-smoke.sh              # defaults: Traefik=8080, direct=10000
#   TRAEFIK_PORT=8080 AUDIT_PORT=10000 ./scripts/e2e-smoke.sh

TRAEFIK_PORT="${TRAEFIK_PORT:-8080}"
AUDIT_PORT="${AUDIT_PORT:-10000}"
CONSUL_PORT="${CONSUL_PORT:-8500}"
TRAEFIK_API_PORT="${TRAEFIK_API_PORT:-8081}"
PROMETHEUS_PORT="${PROMETHEUS_PORT:-9090}"

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
RESET='\033[0m'

PASS=0
FAIL=0

inc_pass() { PASS=$((PASS + 1)); }
inc_fail() { FAIL=$((FAIL + 1)); }

check() {
  local name="$1" url="$2" expected_code="${3:-200}" body_contains="${4:-}"
  printf "  %-50s " "$name"

  resp=$(curl -s -w '\n%{http_code}' "$url" 2>/dev/null) || { echo -e "${RED}FAIL (curl error)${RESET}"; inc_fail; return; }
  code=$(echo "$resp" | tail -1)
  body=$(echo "$resp" | sed '$d')

  if [[ "$code" != "$expected_code" ]]; then
    echo -e "${RED}FAIL (HTTP $code, expected $expected_code)${RESET}"
    inc_fail
    return
  fi

  if [[ -n "$body_contains" && "$body" != *"$body_contains"* ]]; then
    echo -e "${RED}FAIL (body missing: $body_contains)${RESET}"
    inc_fail
    return
  fi

  echo -e "${GREEN}PASS${RESET}"
  inc_pass
}

check_prom_target() {
  local job="$1"
  printf "  %-50s " "prometheus target: $job"

  health=$(curl -s "http://localhost:${PROMETHEUS_PORT}/api/v1/targets" 2>/dev/null \
    | python3 -c "import sys,json; d=json.load(sys.stdin); [print(t['health']) for t in d['data']['activeTargets'] if t['labels']['job']=='$job']" 2>/dev/null)

  if [[ "$health" == "up" ]]; then
    echo -e "${GREEN}PASS${RESET}"
    inc_pass
  else
    echo -e "${RED}FAIL (health=$health)${RESET}"
    inc_fail
  fi
}

echo -e "${CYAN}=== Platform E2E Smoke Test ===${RESET}"
echo ""

echo -e "${CYAN}[1/5] Traefik proxy (port $TRAEFIK_PORT)${RESET}"
check "GET /v1/audit/events" "http://localhost:${TRAEFIK_PORT}/v1/audit/events" 200 "events"
check "GET /v1/audit/events:count" "http://localhost:${TRAEFIK_PORT}/v1/audit/events:count" 200 "totalCount"
echo ""

echo -e "${CYAN}[2/5] Direct audit service (port $AUDIT_PORT)${RESET}"
check "GET /v1/audit/events (direct)" "http://localhost:${AUDIT_PORT}/v1/audit/events" 200 "events"
check "GET /v1/audit/events:count (direct)" "http://localhost:${AUDIT_PORT}/v1/audit/events:count" 200 "totalCount"
check "GET /metrics" "http://localhost:${AUDIT_PORT}/metrics" 200 "go_gc_duration_seconds"
echo ""

echo -e "${CYAN}[3/5] Consul service registry (port $CONSUL_PORT)${RESET}"
check "GET /v1/catalog/services" "http://localhost:${CONSUL_PORT}/v1/catalog/services" 200 "audit.service"
echo ""

echo -e "${CYAN}[4/5] Traefik route discovery (port $TRAEFIK_API_PORT)${RESET}"
check "GET /api/http/routers (audit-api)" "http://localhost:${TRAEFIK_API_PORT}/api/http/routers" 200 "audit-api@consulcatalog"
echo ""

echo -e "${CYAN}[5/5] Prometheus scrape targets${RESET}"
check_prom_target "audit-service"
check_prom_target "otel-collector"
check_prom_target "loki"
check_prom_target "jaeger"
check_prom_target "grafana"
check_prom_target "traefik"
echo ""

echo -e "${CYAN}===============================${RESET}"
echo -e "Results: ${GREEN}$PASS passed${RESET}, ${RED}$FAIL failed${RESET}"

if [[ $FAIL -gt 0 ]]; then
  exit 1
fi
