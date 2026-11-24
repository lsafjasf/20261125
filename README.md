# 20261125

Transactional multi-language media repackager. Pure Go standard library,
no third-party dependencies.

## Problem

The previous packager had two defects when merging per-language source
packs:

- Switching languages restarted the picture because video tracks differed
  across sources instead of sharing one identical timeline.
- Subtitle timing was attached to another language's audio track, so cues
  drifted away from the audio they belong to.

The replacement performs the whole repack as one transaction with full
cross-track validation and automatic rollback.

## Guarantees

- **Atomic commit**: output is written to a same-directory temp file,
  re-read and checksum-verified, then renamed into place. Any failure
  restores the previous output byte-exact (or removes a newly created one);
  no temp/backup files are left behind.
- **Audio check**: every source has one language, strictly ordered audio
  samples whose span matches the video span exactly; duplicate language
  codes across inputs are rejected.
- **Subtitle check**: subtitle language must match the source's audio
  language, and every cue must be inside and aligned to a sample on that
  same audio track (the old cross-track wiring is impossible).
- **Chapter check**: all sources carry identical chapter markers (time and
  content); a single shared chapter track is emitted.
- **Picture check**: every source must contain byte-identical video frames
  at identical timestamps and dimensions; one shared video track is emitted,
  so language switching cannot restart playback.
- **Read-only inputs**: inputs are opened `O_RDONLY`; test suites assert
  their bytes never change, including with `0444` permissions.
- **Path safety**: relative `-in`/`-out` paths are cleaned and confined to
  `-base` (default cwd); `..` escapes, duplicate input spellings, and an
  output aliasing an input are all rejected before any write.

## Container format (`MCPACK`)

All integers big-endian. File = fixed header (magic `MCPACK`, version,
box count, CRC32-C) followed by boxes. Each box holds type
(`VIDE`/`AUDI`/`SUBT`/`CHAP`), language, start/end in microseconds,
dimensions, payload, and a CRC32-C covering the whole box. Reads verify
every checksum and reject trailing bytes.

## Usage

```sh
go build ./...
go test ./...

go run ./cmd/repack gen-testdata -dir ./sample
go run ./cmd/repack repack \
  -in sample/pack-en.mcpack -in sample/pack-fr.mcpack \
  -out merged.mcpack -base .
go run ./cmd/repack verify -pack merged.mcpack
```

Fault injection for rollback testing: `-fail-at
after_write|before_rename|after_rename|verify_output` and `-corrupt-temp`
(flips a byte before verification).

## Layout

- `container.go` — MCPACK reader/writer with CRC32-C verification
- `repack.go` — path resolution, per-source validation, cross-source checks
- `transaction.go` — temp-file commit, verification, rename, rollback
- `sample.go` — deterministic sample pack generator
- `cmd/repack/main.go` — CLI (`repack`, `verify`, `gen-testdata`)
- `*_test.go` — 26 tests covering rollback, read-only inputs, duplicate
  language codes, relative-path variants/escapes, and both legacy defects
