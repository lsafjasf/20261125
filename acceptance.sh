#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"
go test ./...
exec go run ./cmd/mediaclock-acceptance "$@"
