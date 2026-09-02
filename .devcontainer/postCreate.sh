#!/usr/bin/env bash
# Installs the toolchain the quality gates need. Runs once, on container create.
set -euo pipefail

echo "Installing Go tooling..."
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
go install golang.org/x/vuln/cmd/govulncheck@latest
go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0
go install github.com/pressly/goose/v3/cmd/goose@latest
# Pinned, and CI pins the same version: sqlc output differs between releases, so
# a version skew here shows up as a CI diff nobody can reproduce locally. A test
# asserts the two pins agree.
go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1

echo "Installing web dependencies..."
npm --prefix apps/web ci

echo
echo "Ready. Unlike the Windows host, this container has a C compiler, so the"
echo "race detector works here:  make test-race"
