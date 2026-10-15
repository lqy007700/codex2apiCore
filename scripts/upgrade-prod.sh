#!/usr/bin/env bash
# Defaults match the existing deployment supplied by the operator.
set -euo pipefail
script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
exec bash "$script_dir/upgrade.sh" \
  --file "${CORE_COMPOSE_FILE:-/opt/codex2api/docker-compose.prod.yml}" \
  --project "${CORE_COMPOSE_PROJECT:-x-x-codex2api}" \
  --service "${CORE_COMPOSE_SERVICE:-codex2api}" "$@"
