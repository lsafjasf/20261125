# dpxscan

Reconstructs the global frame order of a DPX sequence delivery from
image headers (SMPTE 268M) and delivery manifests — never from file
names. Frame numbers, reel numbers, and reel timecode may all restart
after a reel change; `dpxscan` chains reels by timecode continuity and
reports what needs human attention.

## What it detects

- Duplicate frames (same ordering key, or identical MD5 across files)
- Gaps (missing frame positions / timecode frames within a reel)
- Timecode regressions, including cross-reel rollbacks at reel changes
- Resolution changes across the delivery
- Manifest entries with no file on disk, and (with `--verify-md5`) checksum mismatches

## Output

- Playable ranges: maximal contiguous, constant-resolution runs in global order
- Splice suggestions: how to join ranges manually (bridge gaps with slug,
  drop duplicates, conform mixed rasters, re-strip timeline TC after a rollback)

## Usage

```
go build ./cmd/dpxscan
dpxscan [--fps 24] [--manifest delivery.md5]...--manifest list.csv] \
        [--reel-order Reel_01,Reel_02] [--verify-md5] <delivery-root>
```

- Reels are delivery subdirectories. Without `--reel-order`, reels are
  chained by timecode continuity; unchainable reels are appended in name
  order and flagged with a `reel-order-heuristic` warning.
- Manifests: `md5sum` format or CSV with a header row (`file`/`path`,
  optional `md5`, `size` columns).
- Ordering keys come from the DPX TV-header timecode (BCD `0xHHMMSSFF`)
  when present, otherwise the film-header frame position.

## Demo

```
go run ./cmd/dpxgen /tmp/demo-delivery   # synthetic messy delivery
go run ./cmd/dpxscan /tmp/demo-delivery
```

## Layout

- `internal/dpx` — DPX generic/film/TV header parser (big- and little-endian)
- `internal/analyze` — scanning, manifest parsing, ordering, detection, reporting
- `cmd/dpxscan` — CLI
- `cmd/dpxgen` — synthetic fixture generator
