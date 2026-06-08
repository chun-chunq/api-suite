#!/usr/bin/env bash
# stop.sh — gracefully stop all APIs
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
echo "Stopping all APIs..."
docker compose down
echo "Done. Data volumes are preserved (redis persistence)."
echo "To also remove volumes: docker compose down -v"
