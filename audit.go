package cbarcheck

import (
	"fmt"
	"math"
	"sort"
)

const (
	candidateTie    = 1e-9
	equivalentScore = 0.05
	mismatchScore   = 0.25
	illegalPenalty  = 12.0
)

// Analyze checks frame against target and compares every region with all
// supported range/matrix/depth hypotheses. Violations are evaluated against
// target only; the candidate search is diagnostic and never excuses a target
// failure. Pixels are visited in row-major order, so FirstViolation is the
// first failing pixel in that order.
func Analyze(frame *Frame, layout Layout, target Config, thresholds Thresholds) (*Report, error) {
	if err := frame.Validate(); err != nil {
		return nil, err
	}
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	if frame.Width != layout.Width || frame.Height != layout.Height {
		return nil, fmt.Errorf("frame %dx%d does not match layout %dx%d",
			frame.Width, frame.Height, layout.Width, layout.Height)
	}
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("target: %w", err)
	}
	if err := thresholds.Validate(); err != nil {
		return nil, err
	}

	report := &Report{
		Schema: Schema, FrameWidth: frame.Width, FrameHeight: frame.Height,
		StorageDepth: frame.Depth, Target: target, Thresholds: thresholds,
	}
	for i, region := range layout.Regions {
		rr, err := analyzeRegionCandidates(frame, region, target)
		if err != nil {
			return nil, fmt.Errorf("region %d %q: %w", i, region.Name, err)
		}
		report.Regions = append(report.Regions, rr)
	}
	if err := scanPixels(frame, layout, target, thresholds, report); err != nil {
		return nil, err
	}
	report.GlobalCandidates = aggregateCandidates(report.Regions, target)
	s, err := summarize(report)
	if err != nil {
		return nil, err
	}
	report.Summary = s
	return report, nil
}

func analyzeRegionCandidates(frame *Frame, region Region, target Config) (RegionReport, error) {
	rr := RegionReport{
		Index: region.Index, Name: region.Name, Kind: region.Kind,
		X0: region.X0, Y0: region.Y0, X1: region.X1, Y1: region.Y1,
		RGB: region.RGB, Pixels: region.Width() * region.Height(),
	}
	expected, err := EncodeRGB(region.RGB, target)
	if err != nil {
		return rr, err
	}
	rr.Expected = expected

	for _, cfg := range Configs() {
		c := evaluateCandidate(frame, region, cfg)
		c.IsTarget = cfg == target
		rr.Candidates = append(rr.Candidates, c)
	}
	sortCandidates(rr.Candidates, target)
	best := rr.Candidates[0]
	rr.Best, rr.BestScore = best.Config, best.Score
	for i := range rr.Candidates {
		rr.Candidates[i].Rank = i + 1
		rr.Candidates[i].EquivalentToBest = rr.Candidates[i].Score <= best.Score+equivalentScore
		if rr.Candidates[i].IsTarget {
			rr.TargetScore = rr.Candidates[i].Score
			rr.TargetEquivalentBest = rr.Candidates[i].EquivalentToBest
		}
	}
	return rr, nil
}

func evaluateCandidate(frame *Frame, region Region, cfg Config) CandidateResult {
	c := CandidateResult{Config: cfg}
	expected, err := FromRGB(region.RGB, cfg.Matrix)
	if err != nil {
		return c
	}
	bounds := Bounds(cfg)
	var sumY, sumCb, sumCr float64
	for y := region.Y0; y < region.Y1; y++ {
		row := y * frame.Width
		for x := region.X0; x < region.X1; x++ {
			i := row + x
			raw := Sample{
				Y: float64(frame.Y[i]), Cb: float64(frame.Cb[i]), Cr: float64(frame.Cr[i]),
			}
			actual, _ := DecodeNormalized(raw, cfg)
			ey := CodeError8(actual.Y-expected.Y, cfg, true)
			ecb := CodeError8(actual.Pb-expected.Pb, cfg, false)
			ecr := CodeError8(actual.Pr-expected.Pr, cfg, false)
			sumY += ey
			sumCb += ecb
			sumCr += ecr
			if !inRange(raw.Y, bounds.YMin, bounds.YMax) {
				c.IllegalLumaPixels++
			}
			if !inRange(raw.Cb, bounds.CbMin, bounds.CbMax) ||
				!inRange(raw.Cr, bounds.CrMin, bounds.CrMax) {
				c.IllegalChromaPixels++
			}
		}
	}
	n := float64(region.Width() * region.Height())
	c.MeanLumaError8 = sumY / n
	c.MeanCbError8 = sumCb / n
	c.MeanCrError8 = sumCr / n
	c.Score = c.MeanLumaError8 + c.MeanCbError8 + c.MeanCrError8 +
		illegalPenalty*(float64(c.IllegalLumaPixels)+float64(c.IllegalChromaPixels))/n
	return c
}

func sortCandidates(candidates []CandidateResult, target Config) {
	order := configOrder()
	sort.SliceStable(candidates, func(i, j int) bool {
		d := candidates[i].Score - candidates[j].Score
		if math.Abs(d) > candidateTie {
			return d < 0
		}
		if candidates[i].IsTarget != candidates[j].IsTarget {
			return candidates[i].IsTarget
		}
		return order[candidates[i].Config] < order[candidates[j].Config]
	})
}

func configOrder() map[Config]int {
	order := make(map[Config]int)
	for i, cfg := range Configs() {
		order[cfg] = i
	}
	return order
}

func aggregateCandidates(regions []RegionReport, target Config) []CandidateResult {
	type acc struct {
		c      CandidateResult
		weight float64
	}
	byConfig := make(map[Config]*acc)
	for _, r := range regions {
		for _, c := range r.Candidates {
			a := byConfig[c.Config]
			if a == nil {
				a = &acc{c: CandidateResult{Config: c.Config, IsTarget: c.Config == target}}
				byConfig[c.Config] = a
			}
			w := float64(r.Pixels)
			a.c.MeanLumaError8 += c.MeanLumaError8 * w
			a.c.MeanCbError8 += c.MeanCbError8 * w
			a.c.MeanCrError8 += c.MeanCrError8 * w
			a.c.IllegalLumaPixels += c.IllegalLumaPixels
			a.c.IllegalChromaPixels += c.IllegalChromaPixels
			a.weight += w
		}
	}
	var out []CandidateResult
	for _, cfg := range Configs() {
		a := byConfig[cfg]
		if a == nil || a.weight == 0 {
			continue
		}
		c := a.c
		c.MeanLumaError8 /= a.weight
		c.MeanCbError8 /= a.weight
		c.MeanCrError8 /= a.weight
		c.Score = c.MeanLumaError8 + c.MeanCbError8 + c.MeanCrError8 +
			illegalPenalty*(float64(c.IllegalLumaPixels)+float64(c.IllegalChromaPixels))/a.weight
		out = append(out, c)
	}
	sortCandidates(out, target)
	for i := range out {
		out[i].Rank = i + 1
		out[i].EquivalentToBest = out[i].Score <= out[0].Score+equivalentScore
	}
	return out
}

func scanPixels(frame *Frame, layout Layout, target Config, th Thresholds, report *Report) error {
	idx, err := regionIndex(layout)
	if err != nil {
		return err
	}
	expected := make([]Sample, len(layout.Regions))
	for i, r := range layout.Regions {
		expected[i], err = EncodeRGB(r.RGB, target)
		if err != nil {
			return err
		}
		rr := &report.Regions[i]
		rr.Pixels = r.Width() * r.Height()
	}
	bounds := Bounds(target)
	frameScale := float64(int(1) << (frame.Depth - 8))
	obsSum := make([]Sample, len(layout.Regions))
	errSum := make([]Sample, len(layout.Regions))

	for y := 0; y < frame.Height; y++ {
		for x := 0; x < frame.Width; x++ {
			i := y*frame.Width + x
			ri := idx[i]
			raw := Sample{Y: float64(frame.Y[i]), Cb: float64(frame.Cb[i]), Cr: float64(frame.Cr[i])}
			exp := expected[ri]
			delta := Sample{Y: raw.Y - exp.Y, Cb: raw.Cb - exp.Cb, Cr: raw.Cr - exp.Cr}
			e8 := Sample{
				Y:  math.Abs(delta.Y) / frameScale,
				Cb: math.Abs(delta.Cb) / frameScale,
				Cr: math.Abs(delta.Cr) / frameScale,
			}
			obsSum[ri].Y += raw.Y
			obsSum[ri].Cb += raw.Cb
			obsSum[ri].Cr += raw.Cr
			errSum[ri].Y += e8.Y
			errSum[ri].Cb += e8.Cb
			errSum[ri].Cr += e8.Cr

			var reasons []string
			if !inRange(raw.Y, bounds.YMin, bounds.YMax) {
				reasons = append(reasons, "illegal_luma")
				report.Summary.IllegalLumaPixels++
				report.Regions[ri].IllegalLumaPixels++
			}
			if !inRange(raw.Cb, bounds.CbMin, bounds.CbMax) ||
				!inRange(raw.Cr, bounds.CrMin, bounds.CrMax) {
				reasons = append(reasons, "illegal_chroma")
				report.Summary.IllegalChromaPixels++
				report.Regions[ri].IllegalChromaPixels++
			}
			if e8.Y > th.Luma {
				reasons = append(reasons, "luma_level")
			}
			if e8.Cb > th.Chroma || e8.Cr > th.Chroma {
				reasons = append(reasons, "chroma_level")
			}
			if len(reasons) == 0 {
				continue
			}

			v := &PixelViolation{
				X: x, Y: y, RegionIndex: ri, Region: layout.Regions[ri].Name,
				Reasons: reasons, Observed: raw, Expected: exp, Delta: delta,
				ErrorCode8: e8, Bounds: bounds,
			}
			hasLuma := contains(reasons, "illegal_luma") || contains(reasons, "luma_level")
			hasChroma := contains(reasons, "illegal_chroma") || contains(reasons, "chroma_level")
			if hasLuma {
				report.Summary.LumaViolationPixels++
				report.Regions[ri].LumaViolationPixels++
				if report.FirstLumaViolation == nil {
					report.FirstLumaViolation = v
				}
			}
			if hasChroma {
				report.Summary.ChromaViolationPixels++
				report.Regions[ri].ChromaViolationPixels++
				if report.FirstChromaViolation == nil {
					report.FirstChromaViolation = v
				}
			}
			if report.Regions[ri].FirstViolation == nil {
				report.Regions[ri].FirstViolation = v
			}
			if report.FirstViolation == nil {
				report.FirstViolation = v
			}
		}
	}

	for i := range report.Regions {
		n := float64(report.Regions[i].Pixels)
		if n == 0 {
			continue
		}
		report.Regions[i].ObservedMean = Sample{
			Y: obsSum[i].Y / n, Cb: obsSum[i].Cb / n, Cr: obsSum[i].Cr / n,
		}
		report.Regions[i].MeanError8 = Sample{
			Y: errSum[i].Y / n, Cb: errSum[i].Cb / n, Cr: errSum[i].Cr / n,
		}
	}
	return nil
}

func summarize(report *Report) (Summary, error) {
	if len(report.GlobalCandidates) == 0 {
		return Summary{}, fmt.Errorf("no candidate results")
	}
	s := report.Summary
	s.Pixels = report.FrameWidth * report.FrameHeight
	s.Regions = len(report.Regions)
	s.Target = report.Target
	best := report.GlobalCandidates[0]
	s.Best, s.BestScore = best.Config, best.Score
	for _, c := range report.GlobalCandidates {
		if c.IsTarget {
			s.TargetScore = c.Score
			break
		}
	}
	s.ConfigurationMismatch = s.TargetScore > s.BestScore+mismatchScore
	for _, r := range report.Regions {
		if !r.TargetEquivalentBest {
			s.RegionMismatches++
		}
	}
	s.Compliant = report.FirstViolation == nil && !s.ConfigurationMismatch
	return s, nil
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
