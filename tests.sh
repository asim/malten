#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
go mod verify
go vet ./...
go test -race -count=1 ./...
# Releases disable CGO; exercise database initialization in that mode too.
CGO_ENABLED=0 go test -count=1 ./...
CGO_ENABLED=0 go build -o "${TMPDIR:-/tmp}/malten-test-$$" .
rm -f "${TMPDIR:-/tmp}/malten-test-$$"
