#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

CENTIAN_BIN="${CENTIAN_BIN:-${REPO_ROOT}/build/centian}"
SUITE_PATH="${SUITE_PATH:-${REPO_ROOT}/tests/integrationtests/taskverification/benchmarks/centian_demo_v1}"
REPEAT="${REPEAT:-10}"
CODEX_CONFIG_PATH="${CODEX_CONFIG_PATH:-}"

run_scenario() {
  local label="$1"
  local agent="$2"
  local model="$3"

  echo
  echo "==> Starting scenario: ${label}"
  echo "    agent=${agent} model=${model} repeat=${REPEAT}"

  local cmd=(
    "${CENTIAN_BIN}"
    benchmark
    run
    --suite "${SUITE_PATH}"
    --agent "${agent}"
    --model "${model}"
    --repeat "${REPEAT}"
  )

  if [[ "${agent}" == "codex" && -n "${CODEX_CONFIG_PATH}" ]]; then
    cmd+=(--codex-config "${CODEX_CONFIG_PATH}")
  fi

  "${cmd[@]}"

  echo "==> Finished scenario: ${label}"
}

if [[ ! -x "${CENTIAN_BIN}" ]]; then
  echo "error: centian binary not found or not executable at ${CENTIAN_BIN}" >&2
  exit 1
fi

if [[ ! -d "${SUITE_PATH}" ]]; then
  echo "error: benchmark suite not found at ${SUITE_PATH}" >&2
  exit 1
fi

echo "Repo root: ${REPO_ROOT}"
echo "Centian binary: ${CENTIAN_BIN}"
echo "Suite: ${SUITE_PATH}"
echo "Repeat count: ${REPEAT}"

if [[ -n "${CODEX_CONFIG_PATH}" ]]; then
  echo "Codex config: ${CODEX_CONFIG_PATH}"
else
  echo "Note: benchmark CLI has no Codex reasoning-effort flag."
  echo "      The codex scenarios below use the selected model only."
  echo "      If you want Codex 'high', set CODEX_CONFIG_PATH to a base Codex config that already carries it."
fi

#run_scenario "claude / haiku" "claude" "haiku"
#run_scenario "gemini / gemini-3.1-pro-preview" "gemini" "gemini-3.1-pro-preview"
# run_scenario "gemini / gemini-3-flash-preview" "gemini" "gemini-3-flash-preview"
#run_scenario "codex / gpt-5.4" "codex" "gpt-5.4"
# run_scenario "codex / gpt-5.4-mini" "codex" "gpt-5.4-mini"
run_scenario "claude / sonnet" "claude" "sonnet"
# run_scenario "claude / opus" "claude" "opus"

echo
echo "All benchmark scenarios finished."
