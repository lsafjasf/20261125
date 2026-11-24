package lutdiff

import (
	"math"
	"sort"
)

// SampleResult records both orders and the naive average for one sample.
type SampleResult struct {
	Sample
	AB            RGB
	BA            RGB
	Avg           RGB
	Dist          float64
	DeltaE        float64
	CodeDelta     float64
	AvgRefCodeErr float64
	AvgRefDeltaE  float64
}

// AnalyzeSample evaluates AB, BA and their average. A->B is treated as the
// specified reference order, so AvgRef* measures the error introduced by
// replacing the real A->B result with the average.
func AnalyzeSample(p *Pipeline, s Sample) SampleResult {
	ab := p.AB(s.Color)
	ba := p.BA(s.Color)
	avg := Average(ab, ba)
	return SampleResult{
		Sample:        s,
		AB:            ab,
		BA:            ba,
		Avg:           avg,
		Dist:          DistRGB(ab, ba),
		DeltaE:        DeltaE76(ab, ba),
		CodeDelta:     MaxCodeDelta8(ab, ba),
		AvgRefCodeErr: MaxCodeDelta8(avg, ab),
		AvgRefDeltaE:  DeltaE76(avg, ab),
	}
}

// AnalyzeSamples evaluates every default sample.
func AnalyzeSamples(p *Pipeline) []SampleResult {
	samples := DefaultSamples()
	out := make([]SampleResult, 0, len(samples))
	for _, s := range samples {
		out = append(out, AnalyzeSample(p, s))
	}
	return out
}

// SweepPoint is one dense RGB-cube evaluation.
type SweepPoint struct {
	C         RGB
	Regions   Region
	AB        RGB
	BA        RGB
	Dist      float64
	DeltaE    float64
	CodeDelta float64
}

// RegionStat holds aggregate disagreement statistics for one region family.
type RegionStat struct {
	Label         string
	Count         int
	MeanDist      float64
	P95Dist       float64
	MaxDist       float64
	MeanDeltaE    float64
	MaxDeltaE     float64
	MaxCodeDelta  float64
	Over3Code     int
	AvgRefOver1   int
	MaxAvgRefCode float64
}

// CubeAnalysis is the result of a uniform cube sweep.
type CubeAnalysis struct {
	Grid            int
	Total           int
	All             RegionStat
	Regions         map[Region]*RegionStat
	Top             []SweepPoint
	GrayAxisMaxCode float64
}

func percentile95(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(0.95*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func buildStat(label string, pts []SweepPoint) RegionStat {
	st := RegionStat{Label: label, Count: len(pts)}
	if len(pts) == 0 {
		return st
	}
	dists := make([]float64, len(pts))
	var sumD, sumDE float64
	for i, pt := range pts {
		dists[i] = pt.Dist
		sumD += pt.Dist
		sumDE += pt.DeltaE
		st.MaxDist = math.Max(st.MaxDist, pt.Dist)
		st.MaxDeltaE = math.Max(st.MaxDeltaE, pt.DeltaE)
		st.MaxCodeDelta = math.Max(st.MaxCodeDelta, pt.CodeDelta)
		if pt.CodeDelta > 3.0 {
			st.Over3Code++
		}
		avgErr := pt.CodeDelta / 2
		st.MaxAvgRefCode = math.Max(st.MaxAvgRefCode, avgErr)
		if avgErr > 1.0 {
			st.AvgRefOver1++
		}
	}
	st.MeanDist = sumD / float64(len(pts))
	st.MeanDeltaE = sumDE / float64(len(pts))
	st.P95Dist = percentile95(dists)
	return st
}

// SweepCube evaluates both orders on a grid^3 uniform sampling of the whole
// RGB cube and aggregates per-region statistics plus the worst points.
func SweepCube(p *Pipeline, grid int) *CubeAnalysis {
	if grid < 2 {
		grid = 2
	}
	total := grid * grid * grid
	points := make([]SweepPoint, 0, total)
	buckets := map[Region][]SweepPoint{
		RegionNeutralBit:   nil,
		RegionSaturatedBit: nil,
		RegionBoundaryBit:  nil,
	}

	den := float64(grid - 1)
	for r := 0; r < grid; r++ {
		for g := 0; g < grid; g++ {
			for b := 0; b < grid; b++ {
				c := RGB{float64(r) / den, float64(g) / den, float64(b) / den}
				ab := p.AB(c)
				ba := p.BA(c)
				pt := SweepPoint{
					C:         c,
					Regions:   Classify(c),
					AB:        ab,
					BA:        ba,
					Dist:      DistRGB(ab, ba),
					DeltaE:    DeltaE76(ab, ba),
					CodeDelta: MaxCodeDelta8(ab, ba),
				}
				points = append(points, pt)
				for bit := range buckets {
					if pt.Regions&bit != 0 {
						buckets[bit] = append(buckets[bit], pt)
					}
				}
			}
		}
	}

	all := buildStat("全部输入", points)
	stats := map[Region]*RegionStat{}
	labels := map[Region]string{
		RegionNeutralBit:   "中性色",
		RegionSaturatedBit: "高饱和色",
		RegionBoundaryBit:  "接近边界值",
	}
	for bit, pts := range buckets {
		st := buildStat(labels[bit], pts)
		stats[bit] = &st
	}

	sort.Slice(points, func(i, j int) bool { return points[i].DeltaE > points[j].DeltaE })
	topN := 12
	if len(points) < topN {
		topN = len(points)
	}

	grayAxisMax := 0.0
	for i := 0; i <= 256; i++ {
		t := float64(i) / 256
		c := RGB{t, t, t}
		grayAxisMax = math.Max(grayAxisMax, MaxCodeDelta8(p.AB(c), p.BA(c)))
	}

	return &CubeAnalysis{
		Grid:            grid,
		Total:           total,
		All:             all,
		Regions:         stats,
		Top:             append([]SweepPoint(nil), points[:topN]...),
		GrayAxisMaxCode: grayAxisMax,
	}
}
