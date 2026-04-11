#!/usr/bin/env bash

set -euo pipefail

CENTIAN_BIN="${CENTIAN_BIN:-$(command -v centian || true)}"
SUITE_PATH="${SUITE_PATH:-tests/integrationtests/taskverification/benchmarks/centian_demo_v1}"
REPEAT="${REPEAT:-10}"
CODEX_CONFIG_PATH="${CODEX_CONFIG_PATH:-}"
CENTIAN_CONFIG_PATH="${CENTIAN_CONFIG_PATH:-}"
TEMPLATE_DIRS="${TEMPLATE_DIRS:-}"

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

  if [[ ( "${agent}" == "codex" || "${agent}" == "codex-ollama" ) && -n "${CODEX_CONFIG_PATH}" ]]; then
    cmd+=(--codex-config "${CODEX_CONFIG_PATH}")
  fi
  if [[ -n "${CENTIAN_CONFIG_PATH}" ]]; then
    cmd+=(--centian-config "${CENTIAN_CONFIG_PATH}")
  fi
  if [[ -n "${TEMPLATE_DIRS}" ]]; then
    IFS=',' read -r -a template_dirs <<< "${TEMPLATE_DIRS}"
    for template_dir in "${template_dirs[@]}"; do
      [[ -n "${template_dir}" ]] && cmd+=(--template-dir "${template_dir}")
    done
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
if [[ -n "${CENTIAN_CONFIG_PATH}" ]]; then
  echo "Centian config: ${CENTIAN_CONFIG_PATH}"
fi
if [[ -n "${TEMPLATE_DIRS}" ]]; then
  echo "Template dirs: ${TEMPLATE_DIRS}"
fi

#run_scenario "claude / haiku" "claude" "haiku"
#run_scenario "gemini / gemini-3.1-pro-preview" "gemini" "gemini-3.1-pro-preview"
# run_scenario "gemini / gemini-3-flash-preview" "gemini" "gemini-3-flash-preview"
#run_scenario "codex / gpt-5.4" "codex" "gpt-5.4"
# run_scenario "codex / gpt-5.4-mini" "codex" "gpt-5.4-mini"
# run_scenario "claude / sonnet" "claude" "sonnet"
run_scenario "claude / opus" "claude" "opus"

echo
echo "All benchmark scenarios finished."
