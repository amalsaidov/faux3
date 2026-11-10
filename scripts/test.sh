#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
exec scripts/go.sh test -race -count=1 ./...
