#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
trap 'docker compose -f compose.test.yaml down -v --remove-orphans' EXIT
docker compose -f compose.test.yaml build
docker compose -f compose.test.yaml up -d --wait faux3
