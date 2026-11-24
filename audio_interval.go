package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

func buildMP4AudioIntervals(r io.ReaderAt, track *parsedTrack, opts Options) []AudioIntervalHash {
	if track.info.Kind != "audio" || len(track.samples) == 0 {
		return nil
	}

	window := opts.AudioIntervalSamples
	if window <= 0 {
		window = DefaultOptions().AudioIntervalSamples
	}
	out := make([]AudioIntervalHash, 0, (len(track.samples)+window-1)/window)

	for start := 0; start < len(track.samples); start += window {
		end := start + window
		if end > len(track.samples) {
			end = len(track.samples)
		}
		group := track.samples[start:end]
		h := sha256.New()
		contiguous := true
		var expected int64
		for i, sample := range group {
			if _, err := io.Copy(h, io.NewSectionReader(r, sample.Offset, sample.Size)); err != nil {
				contiguous = false
			}
			if i == 0 {
				expected = sample.Offset + sample.Size
			} else {
				if sample.Offset != expected {
					contiguous = false
				}
				expected = sample.Offset + sample.Size
			}
		}

		first, last := group[0], group[len(group)-1]
		interval := AudioIntervalHash{
			Key:         fmt.Sprintf("track/%d/audio-interval/%d", track.info.TrackID, len(out)+1),
			TrackID:     track.info.TrackID,
			Source:      FormatMP4,
			Index:       len(out) + 1,
			StartSample: int64(first.Index - 1),
			EndSample:   int64(last.Index),
			StartTicks:  first.DTS,
			EndTicks:    last.DTS + int64(last.Duration),
			SHA256:      hex.EncodeToString(h.Sum(nil)),
		}
		if track.info.Timescale > 0 {
			interval.StartSeconds = float64(interval.StartTicks) / float64(track.info.Timescale)
			interval.EndSeconds = float64(interval.EndTicks) / float64(track.info.Timescale)
		}
		if contiguous {
			interval.Offset = first.Offset
			interval.Size = last.Offset + last.Size - first.Offset
		}
		out = append(out, interval)
	}
	return out
}
