#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
docker run --rm -v "$PWD":/src -w /src -v faux3-gomod:/go/pkg/mod -v faux3-gocache:/root/.cache/go-build golang:1.26 sh -c '
  out=$(gofmt -l .); if [ -n "$out" ]; then echo "gofmt needed:"; echo "$out"; exit 1; fi
  go vet ./...
  go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
'
