#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "ffmpeg not found; Go manifest checks can still run via: go run ./cmd/mp4diag verify-fixtures" >&2
  exit 0
fi

echo "== Go manifest + optional ffmpeg frame-hash oracle =="
go test -run 'TestInjectionManifest|TestFFmpegFrameHashOracle' -v ./...

echo
echo "== ffprobe packet/sync summary =="
for file in injections/0*.mp4; do
  echo "--- $file"
  ffprobe -v error \
    -show_entries packet=pts,pos,size,flags \
    -of compact=p=0:nk=1 "$file" | head -12 || true
done

echo
echo "== framemd5 commands =="
echo "ffmpeg -i testdata/clean.mp4 -f framemd5 -"
echo "ffmpeg -i injections/03-compressed-sample2-display-2-4.mp4 -f framemd5 -"
