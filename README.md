# 20261125

Capability-matrix driven display conversion (Go standard library only).

## Problem

A single converter used to emit one output regardless of the target display.
The capability table now drives tier selection per device from peak
luminance (nits), black level (nits), CIE 1931 gamut primaries and bit depth.

## Capability matrix

| Tier | Transfer | Eligibility (all must hold) |
| --- | --- | --- |
| `hdr-pq` | SMPTE ST.2084 PQ | peak >= 200 nits, bits >= 10, valid gamut |
| `sdr-srgb-device` | IEC 61966 sRGB | valid row, above conditions not met |
| `sdr-srgb-safe` | sRGB, fixed 100 nits / 0 nits / 8-bit | immutable verifiable baseline |

Rows are tried top-down. Every built `Profile` must pass `Verify()` (finite
range, knee inside peak, matching code max, non-singular gamut matrix,
tier-specific transfer/bit-depth rules). A failed verification drops to the
next tier and is recorded.

## Fallback contract

Illegal or incomplete capability rows (negative/non-finite peak, black >=
peak, missing or degenerate primaries, bit depth out of 6..16) never reach
tier construction. `Resolve` returns `Fallback: true`, the exact validation
reasons in `FallbackReasons`, the per-tier audit trail in `Attempts`, and the
hard-coded `sdr-srgb-safe` profile, whose constants are themselves asserted by
`TestSafeProfileSelfVerification`. A valid-but-weak panel selecting an SDR
tier is normal matrix resolution and is deliberately not flagged as fallback.

## Pipeline

linear sRGB-relative RGB -> gamut matrix (XYZ between primaries) ->
out-of-gamut clip -> knee tone map (pass-through below knee, exponential
roll-off of HDR highlights to panel peak) -> black-floor lift ->
PQ or sRGB transfer function -> quantization at the tier's bit depth.

## Run

```sh
go test ./...
go run ./cmd/hdr-demo
```

The demo converts one shared material (six linear swatches incl. a 1000-nit
HDR highlight) on three simulated panels (`A-hdr-oled` 1000 nits / P3 / 10-bit,
`B-sdr-lcd` 300 nits / sRGB / 8-bit, `C-cheap-laptop` 180 nits / sRGB / 6-bit)
and three deliberately broken rows. Human-readable tables print to stdout;
the full auditable report is written to `artifacts/conversion-report.json`.
