#!/usr/bin/env bash
set -euo pipefail

mode="${1:-}"
centian_port="${2:-8576}"
jaeger_port="${3:-16686}"

if [[ -z "${mode}" ]]; then
  mode="all"
fi

if [[ "${mode}" != "all" && "${mode}" != "logging" && "${mode}" != "modification" ]]; then
  echo "Usage: $0 <all|logging|modification> [centian_port] [jaeger_port]"
  exit 2
fi

echo "Warning: auth is disabled, so this endpoint is for local demo/testing only."
echo "No auth header is required (demo config uses auth=false)."
echo

logging_endpoint="http://localhost:${centian_port}/mcp/logging-demo"
modification_endpoint="http://localhost:${centian_port}/mcp/modification-demo"

if [[ "${mode}" == "all" || "${mode}" == "logging" ]]; then
  jaeger_url="http://localhost:${jaeger_port}"
  echo "Checking Jaeger at ${jaeger_url}..."
  curl -fsS "${jaeger_url}" >/dev/null
fi

check_endpoint() {
  local endpoint="$1"
  local label="$2"
  echo "Checking ${label} endpoint at ${endpoint}..."
  local status
  status="$(curl -sS -o /dev/null -w "%{http_code}" \
    -X POST "${endpoint}" \
    -H "Content-Type: application/json" \
    --data '{}')"
  if [[ "${status}" == "000" ]]; then
    echo "${label} endpoint is not reachable."
    exit 1
  fi
  if [[ "${status}" =~ ^5[0-9][0-9]$ ]]; then
    echo "${label} endpoint returned server error HTTP ${status}."
    exit 1
  fi
}

if [[ "${mode}" == "all" || "${mode}" == "logging" ]]; then
  check_endpoint "${logging_endpoint}" "logging-demo"
fi

if [[ "${mode}" == "all" || "${mode}" == "modification" ]]; then
  check_endpoint "${modification_endpoint}" "modification-demo"
fi

echo
if [[ "${mode}" == "all" ]]; then
  echo "Demo smoke checks passed."
elif [[ "${mode}" == "logging" ]]; then
  echo "Logging demo smoke checks passed."
else
  echo "Modification demo smoke checks passed."
fi
echo

if [[ "${mode}" == "all" || "${mode}" == "logging" ]]; then
  echo "Copy this into your agents settings for the logging demo:"
  echo '  "centian-demo-logging": {
    "url": "'${logging_endpoint}'"
  }'
  echo "Tell your agent to 'Use logging-demo-db___query and query for all employees data',"
  echo "then inspect traces in ${jaeger_url}."
  echo
fi

if [[ "${mode}" == "all" || "${mode}" == "modification" ]]; then
  echo "Copy this into your agents settings for the modification demo:"
  echo '  "centian-demo-modification": {
    "url": "'${modification_endpoint}'"
  }'
  echo "Tell your agent to 'Use modification-demo-db___query and query for all data on table sample_data_1'."
  echo "The response text should contain [REDACTED_*] placeholders for emails, SSNs, tokens, etc."
fi
