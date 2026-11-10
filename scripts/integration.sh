#!/bin/sh
# Full real-client integration run: AWS SDK v3 and AWS CLI v2 suites,
# then the restart/kill persistence test and the 128 MiB memory test.
set -eu
cd "$(dirname "$0")/.."
trap 'docker compose -f compose.test.yaml --profile mem down -v --remove-orphans' EXIT
docker compose -f compose.test.yaml --profile mem build
docker compose -f compose.test.yaml up -d --wait faux3
docker compose -f compose.test.yaml run --rm node-tests
docker compose -f compose.test.yaml run --rm cli-tests
sh test/integration/persistence.sh
sh test/integration/memory.sh
