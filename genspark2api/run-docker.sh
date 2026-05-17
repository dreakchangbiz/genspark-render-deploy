#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

if [[ ! -f ".env" ]]; then
  echo "Missing .env"
  echo "1) cp .env.example .env"
  echo "2) edit .env and set GS_COOKIE (at least session_id=...)"
  echo "3) re-run: ./run-docker.sh"
  exit 1
fi

docker compose up -d --build
docker compose ps

echo
echo "Tip: follow logs with: docker compose logs -f --tail=100"
