package analysis

import (
	"fmt"
	"math"
	"sort"

	"forensicaudio/internal/audio"
)

type repeatMatch struct {
	first  int
	second int
	offset int
}

type repeatDetector struct {
	window    int
	hop       int
	minGap    int
	minRegion int
}

// DetectRepeats locates copied sample windows. Windows are quantized PCM
// fingerprints; candidates are verified sample-for-sample, so a match means the
// two regions really contain repeated samples rather than merely similar audio.
func DetectRepeats(w *audio.WAV) []RepeatRegion {
	if w == nil || w.FrameCount == 0 {
		return nil
	}
	d := repeatDetector{
		window:    maxInt(512, int(math.Round(0.060*float64(w.SampleRate)))),
		hop:       maxInt(128, int(math.Round(0.020*float64(w.SampleRate)))),
		minGap:    maxInt(4096, int(math.Round(0.350*float64(w.SampleRate)))),
		minRegion: maxInt(2048, int(math.Round(0.160*float64(w.SampleRate)))),
	}
	if w.FrameCount < d.window+d.minGap || w.FrameCount < d.window*2 {
		return nil
	}
	prefix := energyPrefix(w)
	buckets := make(map[uint64][]int)
	var matches []repeatMatch
	const maxBucket = 32
	for start := 0; start+d.window <= w.FrameCount; start += d.hop {
		if windowRMS(prefix, start, d.window, w.Channels) < 0.001 {
			continue
		}
		h := windowHash(w, start, d.window)
		bucket := buckets[h]
		for _, prev := range bucket {
			if start-prev < d.minGap {
				continue
			}
			if !equalWindows(w, prev, start, d.window) {
				continue
			}
			matches = append(matches, repeatMatch{first: prev, second: start, offset: start - prev})
			if len(matches) > 200000 {
				break
			}
		}
		if len(bucket) < maxBucket {
			buckets[h] = append(bucket, start)
		} else {
			buckets[h] = append(bucket[1:], start)
		}
		if len(matches) > 200000 {
			break
		}
	}
	if len(matches) == 0 {
		return nil
	}
	byOffset := make(map[int][]int)
	for _, m := range matches {
		byOffset[m.offset] = append(byOffset[m.offset], m.first)
	}
	var regions []RepeatRegion
	for offset, starts := range byOffset {
		sort.Ints(starts)
		for i := 0; i < len(starts); {
			first := starts[i]
			last := first
			for i++; i < len(starts); i++ {
				if starts[i]-last <= d.hop*2 {
					last = starts[i]
					continue
				}
				break
			}
			if last-first+d.window < d.minRegion {
				continue
			}
			startSec := float64(first) / float64(w.SampleRate)
			endSec := float64(last+d.window) / float64(w.SampleRate)
			matchStart := startSec + float64(offset)/float64(w.SampleRate)
			matchEnd := endSec + float64(offset)/float64(w.SampleRate)
			if matchEnd > w.Duration+0.001 {
				continue
			}
			regions = append(regions, RepeatRegion{
				Start:      round(startSec, 4),
				End:        round(endSec, 4),
				MatchStart: round(matchStart, 4),
				MatchEnd:   round(matchEnd, 4),
				Frames:     last - first + d.window,
				Similarity: 1,
			})
		}
	}
	regions = dedupeRegions(regions)
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].Start == regions[j].Start {
			return regions[i].End < regions[j].End
		}
		return regions[i].Start < regions[j].Start
	})
	if len(regions) > 24 {
		regions = regions[:24]
	}
	for i := range regions {
		regions[i].ID = fmt.Sprintf("R%d", i+1)
	}
	return regions
}

func energyPrefix(w *audio.WAV) []float64 {
	prefix := make([]float64, w.FrameCount+1)
	for i := 0; i < w.FrameCount; i++ {
		var e float64
		for ch := 0; ch < w.Channels; ch++ {
			v := w.Interleaved[i*w.Channels+ch]
			e += v * v
		}
		prefix[i+1] = prefix[i] + e
	}
	return prefix
}

func windowRMS(prefix []float64, start, length, channels int) float64 {
	if start < 0 || start+length >= len(prefix) || length <= 0 || channels <= 0 {
		return 0
	}
	energy := prefix[start+length] - prefix[start]
	return math.Sqrt(energy / float64(length*channels))
}

func windowHash(w *audio.WAV, start, length int) uint64 {
	const prime = uint64(1099511628211)
	h := uint64(1469598103934665603)
	end := minInt(start+length, w.FrameCount)
	for frame := start; frame < end; frame++ {
		base := frame * w.Channels
		for ch := 0; ch < w.Channels; ch++ {
			h ^= math.Float64bits(w.Interleaved[base+ch])
			h *= prime
		}
	}
	return h
}

func equalWindows(w *audio.WAV, a, b, length int) bool {
	if a < 0 || b < 0 || a+length > w.FrameCount || b+length > w.FrameCount {
		return false
	}
	for frame := 0; frame < length; frame++ {
		for ch := 0; ch < w.Channels; ch++ {
			va := math.Float64bits(w.Interleaved[(a+frame)*w.Channels+ch])
			vb := math.Float64bits(w.Interleaved[(b+frame)*w.Channels+ch])
			if va != vb {
				return false
			}
		}
	}
	return true
}

func dedupeRegions(in []RepeatRegion) []RepeatRegion {
	sort.Slice(in, func(i, j int) bool {
		di := in[i].End - in[i].Start
		dj := in[j].End - in[j].Start
		if di == dj {
			return in[i].Start < in[j].Start
		}
		return di > dj
	})
	var out []RepeatRegion
	for _, r := range in {
		duplicate := false
		for _, existing := range out {
			if overlapRatio(r.Start, r.End, existing.Start, existing.End) > 0.60 ||
				overlapRatio(r.MatchStart, r.MatchEnd, existing.MatchStart, existing.MatchEnd) > 0.60 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, r)
		}
	}
	return out
}

func overlapRatio(a0, a1, b0, b1 float64) float64 {
	lo := math.Max(a0, b0)
	hi := math.Min(a1, b1)
	if hi <= lo {
		return 0
	}
	shorter := math.Min(a1-a0, b1-b0)
	if shorter <= 0 {
		return 0
	}
	return (hi - lo) / shorter
}
