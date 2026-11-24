#!/usr/bin/env bash
# End to end validation for the three corruption groups using real ffmpeg
# generated media. Repaired files and JSON reports land in output/, never
# next to (or on top of) the source samples.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

command -v ffmpeg >/dev/null || { echo "ffmpeg not found"; exit 127; }
command -v ffprobe >/dev/null || { echo "ffprobe not found"; exit 127; }

mkdir -p samples output

echo "==> [1/5] Go tests (synthetic groups + safety + real ffmpeg)"
go test ./...

echo "==> [2/5] Build CLI and fixture generator"
go build -o output/mp4repair .
go build -o output/mkfixtures ./cmd/mkfixtures

if [[ ! -s samples/source.mp4 ]]; then
  echo "==> generating real source media"
  ffmpeg -hide_banner -loglevel error -f lavfi \
    -i "testsrc=duration=2:size=320x240:rate=15" \
    -c:v libx264 -g 15 -bf 2 -pix_fmt yuv420p -y samples/source.mp4
  ffmpeg -hide_banner -loglevel error -f lavfi \
    -i "testsrc=duration=2:size=320x240:rate=15" \
    -c:v libx264 -g 15 -bf 2 -pix_fmt yuv420p -movflags +faststart -y samples/source-fast.mp4
fi

echo "==> [3/5] Create truncate / duplicate / bad-offset samples"
./output/mkfixtures -fast samples/source-fast.mp4 -late samples/source.mp4 -dir samples/cases

echo "==> [4/5] Repair each group into output/"
for name in truncated duplicate badoffsets; do
  ./output/mp4repair \
    -in "samples/cases/${name}.mp4" \
    -out "output/${name}.repaired.mp4" \
    -report-file "output/${name}.report.json" \
    | tee "output/${name}.report.txt"
  echo
done

echo "==> [5/5] Decode every repaired file with ffmpeg"
for name in truncated duplicate badoffsets; do
  ffmpeg -v error -i "output/${name}.repaired.mp4" -f null -
  frames="$(ffprobe -v error -count_frames -select_streams v:0 \
    -show_entries stream=nb_read_frames -of default=nw=1:nk=1 \
    "output/${name}.repaired.mp4")"
  echo "  ${name}: decodes cleanly, decoded frames=${frames}"
done

echo "All three groups validated. Reports: output/*.report.json"
