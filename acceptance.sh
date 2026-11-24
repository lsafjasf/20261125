#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "==> go vet"
go vet ./...

echo "==> go test"
go test ./... -count=1

bin_dir="$(mktemp -d)"
work="$(mktemp -d)"
trap 'rm -rf "$bin_dir" "$work"' EXIT

echo "==> build CLI"
go build -o "$bin_dir/colorcal" ./cmd/colorcal

mkdir -p "$work/input" "$work/private"
echo "==> generate observations and separately held acceptance truth"
"$bin_dir/colorcal" sample \
  -out "$work/input/observations.json" \
  -truth "$work/private/generated-truth.json" \
  -seed 42 >/dev/null

echo "==> fit reads only observations.json"
"$bin_dir/colorcal" fit \
  -input "$work/input/observations.json" \
  -report "$work/report.json" \
  -corrected "$work/corrected.json"

echo "==> compare estimate with known generated parameters"
"$bin_dir/colorcal" verify \
  -truth "$work/private/generated-truth.json" \
  -report "$work/report.json"

echo
echo "ACCEPTANCE OK — joint fit, unreliable-patch flags, coupling diagnostics,"
echo "and separated generated-parameter comparison all passed."
