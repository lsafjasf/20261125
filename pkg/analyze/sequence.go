package analyze

import "hdrmeta/pkg/pq"

// Config controls sequence analysis.
type Config struct {
	HighlightNits float64 // region extraction threshold (default 1000)
	ClipNits      float64 // display clip point in nits (default 1000)
	MinAreaRatio  float64 // minimum region area to count as "qualified" (default 0.001)
}

// DefaultConfig returns the recommended thresholds.
func DefaultConfig() Config {
	return Config{
		HighlightNits: 1000,
		ClipNits:      1000,
		MinAreaRatio:  0.001, // 0.1% of frame
	}
}

// AnalyzeSequence runs per-frame analysis and aggregates static metadata
// candidates. MaxCLL/MaxFALL follow the strict CTA-861.3 definitions;
// robust and area-qualified candidates are provided alongside so a human can
// review whether single-pixel or subtitle highlights inflate the values.
func AnalyzeSequence(frames []*Frame, cfg Config) SequenceStats {
	if cfg.HighlightNits <= 0 {
		cfg = DefaultConfig()
	}
	if len(frames) == 0 {
		return SequenceStats{ClipPointNits: cfg.ClipNits}
	}

	depth := frames[0].BitDepth
	seq := SequenceStats{
		FrameCount:    len(frames),
		BitDepth:      depth,
		ClipPointNits: cfg.ClipNits,
		ClipPointCode: pq.NitsToCode(cfg.ClipNits, depth),
		QualifiedArea: int(cfg.MinAreaRatio * float64(frames[0].PixelCount())),
	}

	for _, f := range frames {
		st := AnalyzeFrame(f, cfg.HighlightNits, cfg.ClipNits, cfg.MinAreaRatio)
		seq.Frames = append(seq.Frames, st)

		if st.MaxNits > seq.MaxCLLNits {
			seq.MaxCLLNits = st.MaxNits
		}
		if st.MeanNits > seq.MaxFALLNits {
			seq.MaxFALLNits = st.MeanNits
		}
		if st.P99_9Nits > seq.RobustCLL {
			seq.RobustCLL = st.P99_9Nits
		}
		for _, r := range st.Regions {
			if r.Qualified && r.PeakNits > seq.RegionCLL {
				seq.RegionCLL = r.PeakNits
			}
		}
		seq.TotalClip += st.ClipPixels
	}
	return seq
}

// Recommendation quantizes a candidate luminance into the conventional
// metadata grid (multiples of 50 below 1000, multiples of 100 above) while
// never rounding below the measured candidate.
func Recommendation(nits float64) int {
	if nits <= 0 {
		return 0
	}
	if nits <= 1000 {
		return int(ceilTo(nits, 50))
	}
	return int(ceilTo(nits, 100))
}

func ceilTo(v, step float64) float64 {
	return float64(int64((v+step-0.0001)/step)) * step
}
