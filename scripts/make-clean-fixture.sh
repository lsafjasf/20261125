#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

# The committed testdata/clean.mp4 is the canonical minimal baseline.
# Regenerating it with another libx264 version may move box/sample offsets;
# mkinject recomputes locations and manifest expectations from the new file.
ffmpeg -hide_banner -loglevel error -y \
  -f lavfi -i 'testsrc=size=64x64:rate=10' \
  -frames:v 8 \
  -c:v libx264 -pix_fmt yuv420p \
  -g 4 -keyint_min 4 -sc_threshold 0 \
  -bf 0 -refs 1 -crf 42 -preset ultrafast \
  -movflags +faststart \
  testdata/clean.mp4

go run ./cmd/mkinject
