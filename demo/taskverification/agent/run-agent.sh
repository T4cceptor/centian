#!/bin/bash
set -euo pipefail

ARTIFACTS_DIR="${ARTIFACTS_DIR:-/artifacts}"
CENTIAN_URL="${CENTIAN_URL:-http://centian:8080/mcp/taskverification}"
PROMPT_PATH="${PROMPT_PATH:-/agent/prompts/problem_score_parentheses.md}"
CODEX_HOME_DIR="${CODEX_HOME_DIR:-/tmp/codex-home}"
CODEX_WORKDIR="${CODEX_WORKDIR:-/tmp/codex-workdir}"

mkdir -p "${ARTIFACTS_DIR}" "${ARTIFACTS_DIR}/logs" "${CODEX_HOME_DIR}" "${CODEX_WORKDIR}"
export CODEX_HOME="${CODEX_HOME_DIR}"

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
  echo "OPENAI_API_KEY is required" | tee "${ARTIFACTS_DIR}/agent-error.log"
  exit 1
fi

ready=0
for _ in $(seq 1 60); do
  if (echo > /dev/tcp/centian/8080) >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done

if [[ "${ready}" != "1" ]]; then
  echo "Centian endpoint did not become reachable in time" | tee "${ARTIFACTS_DIR}/agent-error.log"
  exit 1
fi

printf '%s' "${OPENAI_API_KEY}" | codex login --with-api-key >"${ARTIFACTS_DIR}/codex-login.log" 2>&1
codex mcp add centian --url "${CENTIAN_URL}" >"${ARTIFACTS_DIR}/codex-mcp-add.log" 2>&1

MODEL_ARGS=()
if [[ -n "${CODEX_MODEL:-}" ]]; then
  MODEL_ARGS+=(--model "${CODEX_MODEL}")
fi

codex exec \
  --skip-git-repo-check \
  --dangerously-bypass-approvals-and-sandbox \
  --json \
  -C "${CODEX_WORKDIR}" \
  -o "${ARTIFACTS_DIR}/final_message.txt" \
  "${MODEL_ARGS[@]}" \
  - <"${PROMPT_PATH}" \
  >"${ARTIFACTS_DIR}/codex-events.jsonl" \
  2>"${ARTIFACTS_DIR}/codex.stderr.log"
