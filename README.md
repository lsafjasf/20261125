# 20261125

Repository initialized for 20261125.

## Media clock

This repository contains a standard-library-only media clock fix. The master
clock is derived from the audio endpoint's monotonic hardware sample position,
not from wall-clock timers that can jump or be throttled after pause and
background/foreground transitions.

The core mapping is anchored on every play, pause, rate, seek, and source
transition:

```text
media_time = anchor_media_time + rate *
             (hardware_sample_position - anchor_sample) / device_sample_rate
```

Rate is stored as an exact rational value, so common speeds such as `1.25x`
do not accumulate floating-point error over long timelines. Switching sources
re-anchors source-local PTS, source sample rate, and global timeline position
without resetting the monotonic hardware counter.

Video synchronization waits for early frames and drops stale frames after
foreground recovery. It never runs a periodic global seek; `GlobalSeekCalls`
and `AutomaticGlobalSeeks` remain zero in acceptance.

## Run acceptance

```sh
go test ./...
go run ./cmd/mediaclock-acceptance
```

The acceptance command deterministically exercises a three-hour, 30 fps
composition timeline (324,000 frames) with:

- 48 kHz audio hardware clock;
- a 48 kHz source followed by a 44.1 kHz source switch at 105 minutes;
- `0.5x`, `0.75x`, `1x`, `1.25x`, `1.5x`, and `2x` rates;
- two pause/background cycles, including multi-hour wall-clock jumps;
- one background-playback recovery with stale-frame drops;
- exact three-hour endpoint validation.

The current synthetic run reports maximum presented A/V drift around
`10.417µs`, well below the `2ms` acceptance limit.

## Production adapter

Platform code should implement `AudioHardware` using paired hardware timestamp
and sample-position queries (CoreAudio `AudioTimeStamp`, AAudio timestamp,
WASAPI position, or ALSA avail/delay accounting). The sample position must be
the next/played hardware frame, not compressed bytes or userspace queued bytes.
