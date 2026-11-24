# trackmeta

Per-track colour metadata auditor for multi-track MP4/MOV files that mix
HDR and SDR material. Written in pure Go: it parses the container and the
codec bitstreams itself and never shells out to ffprobe or any other
external probing tool.

## What it checks

- Walks every `trak` and every `stsd` sample description (avc1/hev1/hvc1/
  vp09/av01, ...), not just the first track.
- Reads the container-level `colr` box (nclx/nclc) per track: colour
  primaries, transfer characteristics, matrix coefficients, full-range flag.
- Reads the bitstream truth from the codec config record: H.264 SPS VUI
  (avcC), H.265 SPS VUI (hvcC) or VP9 vpcC colour fields.
- Cross-checks container tags against bitstream VUI field by field; a
  correct file-level/first-track tag never masks a wrong or missing
  per-track tag.

## Report

`trackmeta <file.mp4>` prints a JSON report with per-track metadata
(container colour, bitstream colour, HDR/SDR classification) and an
inconsistency list:

- `missing-colr` (error): track has no colr box; the container-level tag
  does not describe it.
- `colr-vs-bitstream` (error): primaries/transfer/matrix disagree between
  container tag and bitstream VUI.
- `range-mismatch` (error): full-range flag disagrees.
- `legacy-colr` / `range-unknown` (warning): deprecated nclc or missing
  range flag.
- `hdr-sdr-mix` (info): the file mixes HDR and SDR/untagged tracks.
- `first-track-tag-mismatch` (error): a track's actual classification
  contradicts the first track's container-level tag.

## Usage

```sh
go run ./cmd/trackmeta [-strict] file.mp4   # -strict: exit 1 on errors
go run ./cmd/genfixture out.mp4             # synthetic HDR+SDR test file
go test ./...                               # unit + synthetic-file tests
```

## Layout

- `mp4/` — ISO-BMFF box walker, trak/stsd/colr parsing.
- `sps/` — Exp-Golomb bit reader, H.264/H.265 SPS VUI parsers, avcC/hvcC/
  vpcC config record readers.
- `report/` — track-level report building and inconsistency detection.
- `cmd/trackmeta` — CLI. `cmd/genfixture` — synthetic sample generator.
- `testutil/` — programmatic MP4/SPS builders used by tests and genfixture.
