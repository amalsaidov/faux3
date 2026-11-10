#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
exec docker run --rm \
  -v "$PWD":/src -w /src \
  -v faux3-gomod:/go/pkg/mod -v faux3-gocache:/root/.cache/go-build \
  -e CGO_ENABLED="${CGO_ENABLED:-1}" \
  golang:1.26 go "$@"
