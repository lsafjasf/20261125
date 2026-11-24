package analyze

import (
	"math"
	"testing"

	"hdrmeta/pkg/pq"
)

const testDepth = 10

func newFrame(name string, w, h int) *Frame {
	n := w * h
	return &Frame{
		Name:     name,
		Width:    w,
		Height:   h,
		BitDepth: testDepth,
		R:        make([]uint32, n),
		G:        make([]uint32, n),
		B:        make([]uint32, n),
	}
}

func set(f *Frame, x, y int, code uint32) {
	i := y*f.Width + x
	f.R[i], f.G[i], f.B[i] = code, code, code
}

func fill(f *Frame, code uint32) {
	for i := range f.R {
		f.R[i], f.G[i], f.B[i] = code, code, code
	}
}

func TestLumaMatchesPQ(t *testing.T) {
	f := newFrame("f", 2, 2)
	code := pq.NitsToCode(1000, testDepth) // 769
	fill(f, uint32(code))
	st := AnalyzeFrame(f, 1000, 1000, 0.001)
	if math.Abs(st.MaxNits-998.93) > 0.5 {
		t.Fatalf("max nits = %.2f, want ~998.93 (code 769 quantization)", st.MaxNits)
	}
	if math.Abs(st.MeanNits-998.93) > 0.5 {
		t.Fatalf("mean nits = %.2f, want ~998.93", st.MeanNits)
	}
}

func TestBlackFrameNoFalsePositive(t *testing.T) {
	f := newFrame("black", 64, 64)
	// leave everything at code 0
	seq := AnalyzeSequence([]*Frame{f}, DefaultConfig())
	if seq.MaxCLLNits != 0 || seq.MaxFALLNits != 0 {
		t.Fatalf("black frame produced nonzero metadata: CLL=%.4f FALL=%.4f",
			seq.MaxCLLNits, seq.MaxFALLNits)
	}
	if seq.RegionCLL != 0 || seq.RobustCLL != 0 {
		t.Fatalf("black frame produced robust/region candidates: %+v", seq)
	}
	st := seq.Frames[0]
	if len(st.Regions) != 0 {
		t.Fatalf("black frame produced %d highlight regions", len(st.Regions))
	}
	if Recommendation(0) != 0 {
		t.Fatal("Recommendation(0) must be 0")
	}
}

func TestSubtitleSpeckleDoesNotInflateRobustCLL(t *testing.T) {
	w, h := 128, 128
	f := newFrame("subtitle", w, h)
	base := uint32(pq.NitsToCode(100, testDepth)) // 520
	fill(f, base)

	// 2x6 white-ish subtitle glyph = 12 px of 4000 nits on a 16384 px
	// frame (0.073%), below the 0.1% area qualification gate.
	hot := uint32(pq.NitsToCode(4000, testDepth)) // 923
	for y := 100; y < 106; y++ {
		for x := 20; x < 22; x++ {
			set(f, x, y, hot)
		}
	}

	st := AnalyzeFrame(f, 1000, 1000, 0.001)

	// Strict MaxCLL is honestly 4000 nits (one real bright pixel exists).
	if math.Abs(st.MaxNits-4000) > 40 {
		t.Fatalf("strict MaxCLL = %.1f, want ~4000", st.MaxNits)
	}
	// But no region meets the 0.1% area gate (12/16384 = 0.073%).
	var qualified, unqualified int
	for _, r := range st.Regions {
		if r.Qualified {
			qualified++
		} else {
			unqualified++
		}
	}
	if qualified != 0 {
		t.Fatalf("subtitle speckle wrongly qualified: %d qualified regions", qualified)
	}
	if unqualified != 1 {
		t.Fatalf("want exactly 1 unqualified region, got %d", unqualified)
	}

	// Robust p99.9 (brightest 0.1% = 17 px here, only 12 are hot) is
	// diluted toward the 100-nit base, unlike the strict single-pixel max.
	if st.P99_9Nits > 3500 {
		t.Fatalf("p99.9 not robust against subtitle: %.1f", st.P99_9Nits)
	}

	seq := AnalyzeSequence([]*Frame{f}, DefaultConfig())
	if seq.RegionCLL != 0 {
		t.Fatalf("region-qualified CLL should be 0 without qualified region, got %.1f",
			seq.RegionCLL)
	}
	if seq.MaxCLLNits < 3900 {
		t.Fatalf("strict MaxCLL must still record the real bright pixel: %.1f", seq.MaxCLLNits)
	}
	if seq.RobustCLL > 3500 {
		t.Fatalf("robust MaxCLL should reject the subtitle speckle, got %.1f", seq.RobustCLL)
	}
}

func TestLargeOverbrightRegionIsQualified(t *testing.T) {
	w, h := 128, 128
	f := newFrame("overbright", w, h)
	base := uint32(pq.NitsToCode(200, testDepth))
	fill(f, base)

	// 32x32 = 1024 px = 6.25% of frame at 4000 nits, clearly a real area.
	hot := uint32(pq.NitsToCode(4000, testDepth))
	for y := 40; y < 72; y++ {
		for x := 40; x < 72; x++ {
			set(f, x, y, hot)
		}
	}

	seq := AnalyzeSequence([]*Frame{f}, DefaultConfig())
	st := seq.Frames[0]
	var q *Region
	for i := range st.Regions {
		if st.Regions[i].Qualified {
			q = &st.Regions[i]
		}
	}
	if q == nil {
		t.Fatal("large overbright region was not qualified")
	}
	if q.Size != 1024 {
		t.Fatalf("region size = %d, want 1024", q.Size)
	}
	if math.Abs(q.PeakNits-4000) > 40 {
		t.Fatalf("region peak = %.1f, want ~4000", q.PeakNits)
	}
	if math.Abs(seq.RegionCLL-4000) > 40 {
		t.Fatalf("RegionCLL = %.1f, want ~4000", seq.RegionCLL)
	}
	if st.ClipPixels != 1024 {
		t.Fatalf("clip pixels = %d, want 1024", st.ClipPixels)
	}
}

func TestMaxFALLIsFrameAveragePeak(t *testing.T) {
	f1 := newFrame("dim", 32, 32)
	fill(f1, uint32(pq.NitsToCode(100, testDepth)))
	f2 := newFrame("bright", 32, 32)
	fill(f2, uint32(pq.NitsToCode(400, testDepth)))

	seq := AnalyzeSequence([]*Frame{f1, f2}, DefaultConfig())
	want400 := pq.CodeToNits(pq.NitsToCode(400, testDepth), testDepth)
	if math.Abs(seq.MaxFALLNits-want400) > 0.5 {
		t.Fatalf("MaxFALL = %.2f, want %.2f", seq.MaxFALLNits, want400)
	}
	if math.Abs(seq.MaxCLLNits-want400) > 0.5 {
		t.Fatalf("MaxCLL = %.2f, want %.2f", seq.MaxCLLNits, want400)
	}
	if got := Recommendation(seq.MaxCLLNits); got != 450 {
		t.Fatalf("Recommendation = %d, want 450 (never round down from 401.5 nits)", got)
	}
	if got := Recommendation(1234); got != 1300 {
		t.Fatalf("Recommendation(1234) = %d, want 1300", got)
	}
}

func TestClipPointCode(t *testing.T) {
	f := newFrame("f", 8, 8)
	fill(f, uint32(pq.NitsToCode(2000, testDepth))) // 846, above 1000-nit clip
	seq := AnalyzeSequence([]*Frame{f}, DefaultConfig())
	if seq.ClipPointCode != 769 {
		t.Fatalf("clip code = %d, want 769", seq.ClipPointCode)
	}
	if seq.TotalClip != 64 {
		t.Fatalf("total clip = %d, want 64", seq.TotalClip)
	}
}
