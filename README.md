# Matroska lacing parser

Go implementation for decoding Matroska `Block` (`0xA1`) and `SimpleBlock`
(`0xA3`) fixed-size, Xiph, and EBML lacing. It reports every frame's absolute
byte range and timestamp, and keeps scanning after malformed blocks instead of
letting a bad boundary consume the following audio.

## API

```go
import matroska "mkvparse"

block, err := matroska.ParseBlock(payload, matroska.BlockOptions{
    ElementOffset:        1234, // absolute file offset of the Block element
    PayloadOffset:        1239, // absolute file offset of payload[0]
    ClusterTimecode:      1000,
    TimestampScale:       1_000_000,
    DefaultFrameDuration: 20_000_000,
})

result := matroska.Scan(fileBytes, matroska.ScanOptions{})
for _, frame := range result.Frames {
    fmt.Println(frame.Start, frame.End, frame.Timecode, frame.TimeNanos)
}
```

`Frame.Start`/`Frame.End` are absolute, end-exclusive file offsets.
`Frame.Timecode` uses Matroska timestamp-scale ticks; `Frame.TimeNanos` is the
same instant in nanoseconds. Laced-frame timestamps advance by the track's
`DefaultDuration` (or `BlockDuration / frameCount` for `BlockGroup` when no
default exists). Set `BlockOptions.DefaultFrameDuration` to override this.

## Lacing

- Fixed: the remaining payload is split into the declared frame count.
- Xiph: sizes for all but the last frame are accumulated in 255-byte chunks.
- EBML: the first size is an unsigned VINT; later sizes are signed VINT deltas
  biased by `2^(7*n-1)-1`. Negative deltas are valid while the resulting frame
  size stays positive.

## Error handling

Errors returned by `ParseBlock` implement `errors.Is` against these kinds:

| Kind | Meaning |
| --- | --- |
| `ErrFirstFrameSize` | First length is invalid, zero, truncated, or overruns the block |
| `ErrNegativeDelta` | An EBML delta makes the next frame size zero or negative |
| `ErrFrameSize` | A frame size is inconsistent with the remaining block bytes |
| `ErrTruncated` | Lacing metadata or frame data ends prematurely |
| `ErrInvalidVINT` | Malformed EBML variable-size integer |
| `ErrInvalidBlock` | Invalid track number or Block header |
| `ErrUnknownElement` | Unknown/unknown-sized element encountered by the scanner |

`Scan` returns these as `Issue` entries. A bad block is not skipped by trusting
its damaged declared end: the scanner searches forward for the next verifiable
`Cluster`, `BlockGroup`, `Block`, or `SimpleBlock` and marks the issue
`Recovered: true`. Unknown elements with known sizes are skipped; unknown-sized
elements trigger a resync. An EOF-truncated element remains terminal.

## CLI

```sh
go run ./cmd/mkvframes -pretty movie.mkv
go run ./cmd/mkvframes -unknown -frame-duration 20000000 movie.mkv
```

Output is JSON containing `blocks`, flattened `frames`, and `issues`.

## Tests

```sh
go test ./...
```
