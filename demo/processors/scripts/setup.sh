#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
demo_dir="$(cd "${script_dir}/.." && pwd)"
cd "${demo_dir}"

command -v docker >/dev/null 2>&1 || {
  echo "docker is required"
  exit 1
}
docker compose version >/dev/null 2>&1 || {
  echo "docker compose is required"
  exit 1
}
command -v go >/dev/null 2>&1 || {
  echo "go is required (local Centian binary build)"
  exit 1
}
command -v make >/dev/null 2>&1 || {
  echo "make is required"
  exit 1
}
command -v curl >/dev/null 2>&1 || {
  echo "curl is required"
  exit 1
}
echo "Tooling check passed."

if [[ ! -f ".env.local" ]]; then
  cp ".env.example" ".env.local"
  echo "Created demo/.env.local from demo/.env.example"
else
  echo "Found existing demo/.env.local"
fi

echo
echo "Setup complete."
echo "Note: demo proxy auth is disabled; no X-Centian-Auth header is needed."
echo "Warning: this is insecure and only intended for local demo/testing."
echo "Logging demo trace UI: Jaeger at http://localhost:16686 (no login, no DSN needed)."
echo "Gateway endpoints:"
echo "  - http://localhost:8576/mcp/logging-demo"
echo "  - http://localhost:8576/mcp/modification-demo"
echo "Next steps:"
echo "  1) make demo-up"
echo "  2) make demo-test"
echo "  3) make demo-down"
