# 20261125

## SRT rational frame-rate converter

A Go standard-library command line tool for SRT time-base conversion without
floating-point arithmetic. All timestamps, frame rates, scale factors, and
quantization errors are represented with `math/big.Rat`.

### Usage

```sh
go run . \
  -input input.srt \
  -output output.srt \
  -source-rate 24000/1001 \
  -rates 25,24000/1001 \
  -rounding floor \
  -report report.txt
```

- `-input` and `-output` accept `-` for stdin and stdout.
- Rates accept decimals or exact fractions. Prefer `24000/1001` for NTSC;
  literal `23.976` is treated as the decimal rational `23976/1000`.
- `-rounding nearest` rounds to the nearest millisecond and is the default.
- `-rounding floor` reproduces truncating converters whose repeated rounding
  can make later subtitles increasingly early.
- The report records every in/out point, exact rational time, quantized
  millisecond, and quantization error for each conversion step.

### Validation

Each stage checks for:

- in or out points reaching or crossing `24:00:00,000`;
- duplicate in points, out points, and complete in/out ranges;
- zero-duration cues and out-before-in cues.

When the final frame rate equals the source rate, the report compares all
original and round-trip in/out points. A global shift is accepted only when
every delta is exactly identical; non-uniform quantization is reported as
impossible to correct with a global offset.

### Tests

```sh
go test ./...
go vet ./...
```
