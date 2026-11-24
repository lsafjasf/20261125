package coloradapt

import (
	"math"
	"sort"
)

type Result struct {
	Sample              Sample
	Method              Method
	D50XYZ              Vec3
	RoundTripAP0        Vec3
	RoundTripDeltaE     float64
	RoundTripXYZ        Vec3
	RoundTripXYZResid   Vec3
	MaxXYZResidual      float64
	PerceptualDeltaE    float64
	WhitePointDeltaE    float64
	NormalizedWhite     Vec3
	ReferenceD50Lab     Lab
	AP0Residual         Vec3
	MaxAP0Residual      float64
	AdaptedD50White     Vec3
	OutsideUnitCube     bool
	OutsideCubeChannels int
}

type Summary struct {
	Method                Method
	Count                 int
	RoundTripMean         float64
	RoundTripMax          float64
	PerceptualMean        float64
	PerceptualP95         float64
	PerceptualMax         float64
	WhitePointDeltaE      float64
	MaxAP0Residual        float64
	MaxXYZResidual        float64
	OutsideUnitCubeCount  int
	NeutralPerceptualMean float64
	SkinPerceptualMean    float64
	BoundaryPerceptualMax float64
	CubePerceptualMax     float64
}

type GroupSummary struct {
	Key   string
	Count int
	Mean  float64
	P95   float64
	Max   float64
}

func Evaluate(method Method, samples []Sample) []Result {
	return EvaluateAgainst(method, MethodBradford, samples)
}

func EvaluateAgainst(method Method, reference Method, samples []Sample) []Result {
	if len(samples) == 0 {
		return nil
	}
	results := make([]Result, len(samples))
	whiteSourceLab := LabFromXYZ(D50White, D50White)
	for index, sample := range samples {
		d50XYZ := D50XYZFromAP0(sample.AP0, method)
		referenceD50XYZ := D50XYZFromAP0(sample.AP0, reference)
		roundTripAP0 := AP0FromD50XYZ(d50XYZ, method)
		roundTripXYZ := D50XYZFromAP0(roundTripAP0, method)
		d50Lab := LabFromXYZ(d50XYZ, D50White)
		referenceD50Lab := LabFromXYZ(referenceD50XYZ, D50White)
		residual := RelativeDifference(sample.AP0, roundTripAP0)
		xyzResidual := vecDifference(d50XYZ, roundTripXYZ)
		adaptedWhite := D50XYZFromAP0(AP0SampleFromXYZ(ACESWhite), method)
		normalizedWhite := NormalizeWhite(adaptedWhite)

		results[index] = Result{
			Sample:              sample,
			Method:              method,
			D50XYZ:              d50XYZ,
			RoundTripAP0:        roundTripAP0,
			RoundTripDeltaE:     DeltaE2000(d50Lab, LabFromXYZ(roundTripXYZ, D50White)),
			RoundTripXYZ:        roundTripXYZ,
			RoundTripXYZResid:   xyzResidual,
			MaxXYZResidual:      maxAbs(xyzResidual),
			PerceptualDeltaE:    DeltaE2000(referenceD50Lab, d50Lab),
			WhitePointDeltaE:    DeltaE2000(whiteSourceLab, LabFromXYZ(normalizedWhite, D50White)),
			NormalizedWhite:     normalizedWhite,
			ReferenceD50Lab:     referenceD50Lab,
			AP0Residual:         residual,
			MaxAP0Residual:      maxAbs(residual),
			AdaptedD50White:     adaptedWhite,
			OutsideUnitCube:     outsideUnitCube(roundTripAP0),
			OutsideCubeChannels: outsideUnitCubeChannels(roundTripAP0),
		}
	}
	return results
}

func NormalizeWhite(value Vec3) Vec3 {
	if value[1] == 0 {
		return value
	}
	return Vec3{value[0] / value[1], 1, value[2] / value[1]}
}

func Summarize(results []Result) Summary {
	if len(results) == 0 {
		return Summary{}
	}
	roundTrip := make([]float64, len(results))
	perceptual := make([]float64, len(results))
	neutral := make([]float64, 0)
	skin := make([]float64, 0)
	boundary := make([]float64, 0)
	summary := Summary{
		Method: results[0].Method,
		Count:  len(results),
	}

	for index, result := range results {
		roundTrip[index] = result.RoundTripDeltaE
		perceptual[index] = result.PerceptualDeltaE
		summary.RoundTripMax = math.Max(summary.RoundTripMax, result.RoundTripDeltaE)
		summary.PerceptualMax = math.Max(summary.PerceptualMax, result.PerceptualDeltaE)
		summary.WhitePointDeltaE = result.WhitePointDeltaE
		summary.MaxAP0Residual = math.Max(summary.MaxAP0Residual, result.MaxAP0Residual)
		summary.MaxXYZResidual = math.Max(summary.MaxXYZResidual, result.MaxXYZResidual)
		if result.OutsideUnitCube {
			summary.OutsideUnitCubeCount++
		}

		switch result.Sample.Group {
		case "neutral":
			neutral = append(neutral, result.PerceptualDeltaE)
		case "skin":
			skin = append(skin, result.PerceptualDeltaE)
		case "ap0_gamut_edge", "ap0_gamut_near_edge":
			boundary = append(boundary, result.PerceptualDeltaE)
		}
	}

	summary.RoundTripMean = mean(roundTrip)
	summary.PerceptualMean = mean(perceptual)
	summary.PerceptualP95 = percentileOrZero(perceptual, 0.95)
	summary.NeutralPerceptualMean = mean(neutral)
	summary.SkinPerceptualMean = mean(skin)
	summary.BoundaryPerceptualMax = maxSlice(boundary)
	summary.CubePerceptualMax = maxGroup(results, "encoding_cube_vertex", "encoding_cube_edge", "encoding_cube_face")
	return summary
}

func SummarizeGroup(results []Result, key string) GroupSummary {
	values := make([]float64, 0, len(results))
	for _, result := range results {
		if groupKey(result) == key {
			values = append(values, result.PerceptualDeltaE)
		}
	}
	return GroupSummary{
		Key:   key,
		Count: len(values),
		Mean:  mean(values),
		P95:   percentileOrZero(values, 0.95),
		Max:   maxSlice(values),
	}
}

func SummarizeIlluminant(results []Result, illuminant string) GroupSummary {
	values := make([]float64, 0, len(results))
	for _, result := range results {
		if result.Sample.Illuminant == illuminant {
			values = append(values, result.PerceptualDeltaE)
		}
	}
	return GroupSummary{
		Key:   illuminant,
		Count: len(values),
		Mean:  mean(values),
		P95:   percentileOrZero(values, 0.95),
		Max:   maxSlice(values),
	}
}

func GroupKeys(results []Result) []string {
	seen := map[string]bool{}
	var keys []string
	for _, result := range results {
		key := groupKey(result)
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func IlluminantKeys(results []Result) []string {
	seen := map[string]bool{}
	var keys []string
	for _, result := range results {
		key := result.Sample.Illuminant
		if !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func groupKey(result Result) string {
	return result.Sample.Group
}

func vecDifference(want, got Vec3) Vec3 {
	return Vec3{
		got[0] - want[0],
		got[1] - want[1],
		got[2] - want[2],
	}
}

func maxGroup(results []Result, groups ...string) float64 {
	values := make([]float64, 0)
	for _, result := range results {
		for _, group := range groups {
			if result.Sample.Group == group {
				values = append(values, result.PerceptualDeltaE)
			}
		}
	}
	return maxSlice(values)
}

func outsideUnitCube(value Vec3) bool {
	return outsideUnitCubeChannels(value) > 0
}

func outsideUnitCubeChannels(value Vec3) int {
	count := 0
	for _, channel := range value {
		if channel < -1e-10 || channel > 1+1e-10 {
			count++
		}
	}
	return count
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func percentile(values []float64, q float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := int(math.Ceil(q*float64(len(sorted)))) - 1
	index = maxInt(index, 0)
	return sorted[minInt(index, len(sorted)-1)]
}

func percentileOrZero(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return percentile(values, q)
}

func maxAbs(values Vec3) float64 {
	return math.Max(math.Abs(values[0]), math.Max(math.Abs(values[1]), math.Abs(values[2])))
}

func maxSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxValue := values[0]
	for _, value := range values[1:] {
		maxValue = math.Max(maxValue, value)
	}
	return maxValue
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
