package cutluma

import (
	"math"
	"math/rand"
)

// Scenario kind labels shared by detection and ground truth.
const (
	KindConversion  = "conversion_mismatch"
	KindNatural     = "natural_light_change"
	KindMixed       = "mixed"
	KindPreexisting = "preexisting_step"
	KindConsistent  = "consistent"
	KindAmbiguous   = "ambiguous"
)

// CutTruth records the scripted cause of a splice in the synthetic scenario.
type CutTruth struct {
	CutIndex int    `json:"cutIndex"`
	Frame    int    `json:"frame"`
	Kind     string `json:"kind"`
	Note     string `json:"note"`
}

// Scenario is a deterministic ten-segment test scene.
type Scenario struct {
	Job              *Job       `json:"job"`
	Illumination     []float64  `json:"illumination"`
	Truth            []CutTruth `json:"truth"`
	FramesPerSegment int        `json:"framesPerSegment"`
}

func smoothStep01(x float64) float64 {
	x = clampF(x, 0, 1)
	return x * x * (3 - 2*x)
}

func ramp(t float64, start, end float64) float64 {
	return smoothStep01((t - start) / (end - start))
}

// illumination is the latent scene light curve. It contains slow benign
// variation plus scripted natural ramps and one source-side hard jump.
func illumination(t int) float64 {
	x := float64(t)
	e := 0.72 +
		0.020*math.Sin(2*math.Pi*x/520+0.7) +
		0.012*math.Sin(2*math.Pi*x/197+2.1)
	// Natural ramp straddling cut 2 (frame 360).
	e += 0.075 * ramp(x, 332, 388)
	// Mixed natural ramp straddling cut 4 (frame 600).
	e += 0.070 * ramp(x, 574, 626)
	// Broad slow swell straddling cut 6 (frame 840), begins well before it.
	e += 0.110 * ramp(x, 780, 900)
	// Real source-side light switch exactly at cut 8 (frame 1080).
	if x >= 1080 {
		e += 0.130
	}
	return clampF(e, 0.35, 1.20)
}

// segmentParamTable realizes the scripted cut causes:
//
//	cut0 A->B conversion, cut1 B->C conversion, cut2 C==C natural,
//	cut3 C->D conversion, cut4 D->E mixed, cut5 E->F conversion,
//	cut6 F==F natural, cut7 F->G conversion, cut8 G==G preexisting.
func segmentParamTable() []ConvertParams {
	A := ConvertParams{Black: 0.020, White: 0.960, ExposureEV: 0.00, TransferPower: 1.00}
	B := ConvertParams{Black: 0.020, White: 0.960, ExposureEV: 0.22, TransferPower: 1.00}
	C := ConvertParams{Black: 0.035, White: 0.935, ExposureEV: 0.05, TransferPower: 0.96}
	D := ConvertParams{Black: 0.012, White: 0.975, ExposureEV: -0.18, TransferPower: 1.04}
	E := ConvertParams{Black: 0.022, White: 0.955, ExposureEV: 0.12, TransferPower: 0.92}
	F := ConvertParams{Black: 0.018, White: 0.962, ExposureEV: -0.05, TransferPower: 1.06}
	G := ConvertParams{Black: 0.025, White: 0.950, ExposureEV: 0.00, TransferPower: 1.00}
	return []ConvertParams{A, B, C, C, D, E, F, F, G, G}
}

// BuildScenario constructs the deterministic scene. No wall clock or global
// random source is used, so tests and reports are reproducible.
func BuildScenario() *Scenario {
	const segCount = 10
	const perSeg = 120
	const patches = 48
	const reps = 8
	const noiseSigma = 0.006

	params := segmentParamTable()
	frameCount := segCount * perSeg
	cuts := make([]int, segCount-1)
	segments := make([]SegmentJob, segCount)
	for k := 0; k < segCount; k++ {
		start := k * perSeg
		end := start + perSeg
		segments[k] = SegmentJob{Index: k, Start: start, End: end, Params: &params[k]}
		if k < segCount-1 {
			cuts[k] = end
		}
	}

	rng := rand.New(rand.NewSource(20261125))
	levels := QuantileLevels
	frames := make([]FrameRow, frameCount)
	light := make([]float64, frameCount)

	srcSamples := make([]float64, 0, patches*reps)
	outSamples := make([]float64, 0, patches*reps)

	for t := 0; t < frameCount; t++ {
		seg := t / perSeg
		e := illumination(t)
		light[t] = e
		srcSamples = srcSamples[:0]
		outSamples = outSamples[:0]
		for p := 0; p < patches; p++ {
			albedo := 0.03 + 0.87*float64(p)/float64(patches-1)
			for r := 0; r < reps; r++ {
				n := rng.NormFloat64() * noiseSigma
				x := clamp01(albedo*e + 0.010 + n)
				srcSamples = append(srcSamples, x)
				outSamples = append(outSamples, params[seg].Apply(x))
			}
		}
		row := FrameRow{Frame: t, Out: make([]float64, len(levels)), Src: make([]float64, len(levels))}
		for qi, q := range levels {
			row.Out[qi] = Quantile(outSamples, q)
			row.Src[qi] = Quantile(srcSamples, q)
		}
		frames[t] = row
	}

	truthKinds := []string{
		KindConversion, KindConversion, KindNatural,
		KindConversion, KindMixed, KindConversion,
		KindNatural, KindConversion, KindPreexisting,
	}
	truthNotes := []string{
		"EV +0.22 stop drift",
		"black/white point drift",
		"smooth natural ramp straddles cut, identical params",
		"EV -0.18 and power drift",
		"natural ramp plus conversion mismatch",
		"transfer power drift",
		"broad natural swell starting before cut, identical params",
		"white point and power drift",
		"source light hard jump at cut, identical params",
	}
	truth := make([]CutTruth, len(cuts))
	for i, c := range cuts {
		truth[i] = CutTruth{CutIndex: i, Frame: c, Kind: truthKinds[i], Note: truthNotes[i]}
	}

	job := &Job{
		Name:           "ten-segment splice diagnostic scene",
		FrameCount:     frameCount,
		QuantileLevels: append([]float64(nil), levels...),
		Cuts:           cuts,
		Frames:         frames,
		Segments:       segments,
	}
	return &Scenario{Job: job, Illumination: light, Truth: truth, FramesPerSegment: perSeg}
}
