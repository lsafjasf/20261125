package nightwb

import (
	"math"
	"testing"
)

func synthFrame(w, h int, fn func(x, y int) ([3]float64, [3]bool, [3]bool)) *Frame {
	f := &Frame{
		Width:          w,
		Height:         h,
		RGB:            make([][3]float64, w*h),
		ChannelClipped: make([][3]bool, w*h),
		ChannelCrushed: make([][3]bool, w*h),
	}
	i := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rgb, clip, crush := fn(x, y)
			f.RGB[i] = rgb
			f.ChannelClipped[i] = clip
			f.ChannelCrushed[i] = crush
			i++
		}
	}
	return f
}

func applyIlluminant(refl, ill [3]float64) [3]float64 {
	return [3]float64{refl[0] * ill[0], refl[1] * ill[1], refl[2] * ill[2]}
}

func avgChroma(px [][3]float64) float64 {
	var sum float64
	for _, p := range px {
		cR, cB := chroma(p)
		sum += math.Hypot(cR, cB)
	}
	return sum / float64(len(px))
}

// TestMixedLight: half sodium-lit, half cool-neon-lit, gray card in each zone.
func TestMixedLight(t *testing.T) {
	warm := [3]float64{1.0, 0.55, 0.28}
	cool := [3]float64{0.55, 0.75, 1.0}
	gray := [3]float64{0.5, 0.5, 0.5}
	w, h := 120, 120
	f := synthFrame(w, h, func(x, y int) ([3]float64, [3]bool, [3]bool) {
		ill := warm
		if x >= w/2 {
			ill = cool
		}
		var refl [3]float64
		switch {
		case x >= 40 && x < 80 && y >= 40 && y < 80:
			refl = gray
		case (x+y)%7 == 0:
			refl = [3]float64{0.8, 0.2, 0.2}
		case (x+y)%11 == 0:
			refl = [3]float64{0.2, 0.25, 0.8}
		default:
			v := 0.2 + 0.5*float64((x*7+y*13)%17)/17.0
			refl = [3]float64{v, v, v}
		}
		return applyIlluminant(refl, ill), [3]bool{}, [3]bool{}
	})

	p := Run(f)
	ill := p.Report.Illuminant
	if !ill.MixedLight || len(ill.Clusters) != 2 {
		t.Fatalf("expected mixed light with 2 clusters, got mixed=%v clusters=%d",
			ill.MixedLight, len(ill.Clusters))
	}
	if ill.Confidence <= 0 || ill.Confidence > 1 {
		t.Fatalf("confidence out of range: %v", ill.Confidence)
	}
	if ill.Clusters[0].CCT == 0 || ill.Clusters[1].CCT == 0 ||
		ill.Clusters[0].CCT == ill.Clusters[1].CCT {
		t.Fatalf("cluster CCTs not distinguished: %v",
			[]float64{ill.Clusters[0].CCT, ill.Clusters[1].CCT})
	}
	srcNeutral := avgChroma(f.RGB)
	aggNeutral := avgChroma(p.Aggressive.RGB)
	if aggNeutral >= srcNeutral {
		t.Fatalf("aggressive did not reduce mean chroma: src=%.4f agg=%.4f",
			srcNeutral, aggNeutral)
	}
	if p.Report.Schemes[0].OutOfGamutFrac > p.Report.Schemes[1].OutOfGamutFrac+1e-9 {
		t.Fatalf("conservative gamut excursion exceeds aggressive")
	}
}

// TestMonochromaticReflectance: one-hue frame with almost no neutral samples
// must fall back to gray world and flag low confidence.
func TestMonochromaticReflectance(t *testing.T) {
	w, h := 96, 96
	ill := [3]float64{1.0, 0.6, 0.35}
	f := synthFrame(w, h, func(x, y int) ([3]float64, [3]bool, [3]bool) {
		v := 0.2 + 0.6*float64((x+y)%13)/13.0
		refl := [3]float64{v, v * 0.35, v * 0.3}
		return applyIlluminant(refl, ill), [3]bool{}, [3]bool{}
	})

	p := Run(f)
	if p.Report.Illuminant.Method != "grayworld-fallback" {
		t.Fatalf("expected grayworld fallback, got %q", p.Report.Illuminant.Method)
	}
	if p.Report.Illuminant.Confidence >= 0.35 {
		t.Fatalf("expected low confidence, got %.2f", p.Report.Illuminant.Confidence)
	}
	if p.Stats.LowConfFrac < 0.99 {
		t.Fatalf("expected nearly all pixels low-confidence, got %.3f", p.Stats.LowConfFrac)
	}
}

// TestExposureAnomalies covers clipped highlights and crushed shadows.
func TestExposureAnomalies(t *testing.T) {
	w, h := 96, 96
	ill := [3]float64{1.0, 0.5, 0.3}
	over := synthFrame(w, h, func(x, y int) ([3]float64, [3]bool, [3]bool) {
		refl := [3]float64{0.5, 0.5, 0.5}
		px := applyIlluminant(refl, ill)
		clip := [3]bool{}
		if y < h/6 {
			px = [3]float64{1, 1, 1}
			clip = [3]bool{true, true, true}
		}
		return px, clip, [3]bool{}
	})
	pOver := Run(over)
	if pOver.Report.Exposure.ExposureState != "overexposed" {
		t.Fatalf("expected overexposed, got %q", pOver.Report.Exposure.ExposureState)
	}
	if pOver.Stats.ClippedFrac < 0.1 {
		t.Fatalf("clipped fraction too low: %.3f", pOver.Stats.ClippedFrac)
	}

	under := synthFrame(w, h, func(x, y int) ([3]float64, [3]bool, [3]bool) {
		crush := [3]bool{}
		px := [3]float64{0.01, 0.008, 0.006}
		if y < h/4 {
			px = [3]float64{0, 0, 0}
			crush = [3]bool{true, true, true}
		}
		return px, [3]bool{}, crush
	})
	pUnder := Run(under)
	if pUnder.Report.Exposure.ExposureState != "underexposed" {
		t.Fatalf("expected underexposed, got %q", pUnder.Report.Exposure.ExposureState)
	}
	if pUnder.Stats.CrushedFrac < 0.2 {
		t.Fatalf("crushed fraction too low: %.3f", pUnder.Stats.CrushedFrac)
	}
}

// TestAdaptationNeutralizesGrayCard: aggressive fully neutralizes, conservative
// deliberately retains a residual cast.
func TestAdaptationNeutralizesGrayCard(t *testing.T) {
	ill := [3]float64{1.0, 0.55, 0.32}
	w, h := 64, 64
	f := synthFrame(w, h, func(x, y int) ([3]float64, [3]bool, [3]bool) {
		v := 0.3 + 0.4*float64((x+y)%9)/9.0
		refl := [3]float64{v, v, v}
		return applyIlluminant(refl, ill), [3]bool{}, [3]bool{}
	})
	p := Run(f)

	src := avgChroma(f.RGB)
	agg := avgChroma(p.Aggressive.RGB)
	cons := avgChroma(p.Conservative.RGB)
	if agg > 0.02 {
		t.Fatalf("aggressive residual chroma too high: %.4f", agg)
	}
	if !(cons < src) {
		t.Fatalf("conservative did not reduce chroma: src=%.4f cons=%.4f", src, cons)
	}
	if !(cons > agg) {
		t.Fatalf("conservative should retain more cast: cons=%.4f agg=%.4f", cons, agg)
	}
}

func TestCCTSanity(t *testing.T) {
	warm := SRGBToXYZ.mulVec([3]float64{1, 0.6, 0.35})
	cool := SRGBToXYZ.mulVec([3]float64{0.6, 0.8, 1})
	cctW, _ := XYZToCCT(warm[0], warm[1], warm[2])
	cctC, _ := XYZToCCT(cool[0], cool[1], cool[2])
	if cctW == 0 || cctC == 0 || cctW >= cctC {
		t.Fatalf("CCT ordering wrong: warm=%.0f cool=%.0f", cctW, cctC)
	}
}
