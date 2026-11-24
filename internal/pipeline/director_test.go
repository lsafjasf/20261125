package pipeline

import (
	"math"
	"testing"

	"rec2020-to-rec709-shoulder/internal/colorconv"
)

func lowStats(frame int) FrameStats {
	return FrameStats{
		Frame: frame, Pixels: 1000,
		MeanLuma: .25, P95Max: .55, P995Max: .82,
		FractionOver1: 0, FractionNearHighlight: .025,
		HighlightPixels: .01, HighlightDetailRatio: .7,
		LumaHistogram: []float64{1},
	}
}

func highStats(frame int) FrameStats {
	return FrameStats{
		Frame: frame, Pixels: 1000,
		MeanLuma: .7, P95Max: 1.8, P995Max: 2.4,
		FractionOver1: .045, FractionNearHighlight: .38,
		HighlightPixels: .22, HighlightDetailRatio: 1.9,
		LumaHistogram: []float64{0, 1},
	}
}

func noisyStats(frame int) FrameStats {
	s := highStats(frame)
	s.FractionOver1 = .8
	s.P995Max = 9
	s.FractionNearHighlight = .95
	s.HighlightDetailRatio = 20
	return s
}

func TestAdaptiveRejectsSingleFrameNoise(t *testing.T) {
	stats := make([]FrameStats, 7)
	for i := range stats {
		stats[i] = lowStats(i)
	}
	stats[3] = noisyStats(3)
	decisions := BuildDecisions(stats, DefaultAdaptiveOptions())

	noiseDecision := decisions[3]
	if noiseDecision.Pressure > .25 {
		t.Fatalf("single-frame noise leaked into stabilized pressure: %.2f", noiseDecision.Pressure)
	}
	if noiseDecision.Profile == ProfileTexture {
		t.Fatalf("single noisy frame changed profile to %s", noiseDecision.Profile)
	}
	for i := 1; i < len(decisions); i++ {
		delta := math.Abs(decisions[i].Knee - decisions[i-1].Knee)
		if delta > DefaultAdaptiveOptions().MaxSlew+1e-9 {
			t.Fatalf("knee jumped %.3f between frames %d-%d", delta, i-1, i)
		}
	}
}

func TestAdaptiveRespondsToPersistentScene(t *testing.T) {
	stats := make([]FrameStats, 10)
	for i := range stats {
		if i < 4 {
			stats[i] = lowStats(i)
		} else {
			stats[i] = highStats(i)
		}
	}
	opt := DefaultAdaptiveOptions()
	decisions := BuildDecisions(stats, opt)
	last := decisions[len(decisions)-1]
	if last.Profile != ProfileTexture {
		t.Fatalf("persistent bright detail scene did not enter texture profile: %s pressure %.2f", last.Profile, last.Pressure)
	}
	if last.Knee >= .70 {
		t.Fatalf("texture profile did not lower knee enough: %.3f", last.Knee)
	}
	for i := 1; i < len(decisions); i++ {
		delta := math.Abs(decisions[i].Knee - decisions[i-1].Knee)
		if delta > opt.MaxSlew+1e-9 {
			t.Fatalf("knee jumped %.3f between frames %d-%d", delta, i-1, i)
		}
	}
}

func TestHueDeltaWraps(t *testing.T) {
	cases := [][3]float64{{350, 10, 20}, {10, 350, -20}, {170, -170, 20}}
	for _, tc := range cases {
		got := colorconv.HueDelta(tc[0], tc[1])
		if math.Abs(got-tc[2]) > 1e-9 {
			t.Fatalf("hue delta %.1f->%.1f = %.1f, want %.1f", tc[0], tc[1], got, tc[2])
		}
	}
}
