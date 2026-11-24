package pipeline

import (
	"fmt"
	"math"
	"sort"
)

const (
	ProfileSpecular = "specular_guard"
	ProfileBalanced = "balanced"
	ProfileTexture  = "texture_guard"
)

type AdaptiveOptions struct {
	Window  int
	MaxSlew float64
}

type StableFeatures struct {
	MeanLuma              float64 `json:"mean_luma"`
	P995Max               float64 `json:"p995_max_rgb"`
	FractionOver1         float64 `json:"fraction_over_1"`
	FractionNearHighlight float64 `json:"fraction_highlight_area"`
	HighlightPixels       float64 `json:"highlight_pixels"`
	HighlightDetailRatio  float64 `json:"highlight_detail_ratio"`
}

type TriggerConditions struct {
	OverRange             bool    `json:"over_range"`
	OverRangePct          float64 `json:"over_range_pct"`
	OverRangeThreshold    float64 `json:"over_range_threshold_pct"`
	Headroom              bool    `json:"headroom"`
	P995Max               float64 `json:"p995_max"`
	P995Threshold         float64 `json:"p995_threshold"`
	HighlightArea         bool    `json:"highlight_area"`
	HighlightAreaPct      float64 `json:"highlight_area_pct"`
	AreaThresholdPct      float64 `json:"area_threshold_pct"`
	HighlightTexture      bool    `json:"highlight_texture"`
	DetailRatio           float64 `json:"detail_ratio"`
	DetailThreshold       float64 `json:"detail_threshold"`
	Pressure              float64 `json:"pressure"`
	TextureEnterPressure  float64 `json:"texture_enter_pressure"`
	SpecularEnterPressure float64 `json:"specular_enter_pressure"`
}

type Decision struct {
	Frame    int               `json:"frame"`
	Profile  string            `json:"profile"`
	Knee     float64           `json:"knee"`
	RawKnee  float64           `json:"raw_knee"`
	Pressure float64           `json:"pressure"`
	Window   int               `json:"temporal_median_window"`
	MaxSlew  float64           `json:"max_knee_slew_per_frame"`
	SceneCut bool              `json:"scene_cut"`
	CutScore float64           `json:"scene_cut_score"`
	Raw      FrameStats        `json:"raw_stats"`
	Stable   StableFeatures    `json:"stable_features"`
	Triggers TriggerConditions `json:"triggers"`
	Reasons  []string          `json:"reasons"`
}

func DefaultAdaptiveOptions() AdaptiveOptions {
	return AdaptiveOptions{Window: 5, MaxSlew: 0.035}
}

func BuildDecisions(stats []FrameStats, opt AdaptiveOptions) []Decision {
	if opt.Window < 3 {
		opt.Window = 3
	}
	if opt.Window%2 == 0 {
		opt.Window++
	}
	if opt.MaxSlew <= 0 {
		opt.MaxSlew = 0.035
	}

	decisions := make([]Decision, 0, len(stats))
	state := ProfileBalanced
	previousKnee := 0.78
	strongRun := 0
	quietRun := 0

	for i := range stats {
		stable := medianFeatures(stats, i, opt.Window)
		pressure := highlightPressure(stable)
		rawKnee := clamp(0.86-0.24*pressure, 0.62, 0.86)
		triggers := evaluateTriggers(stable, pressure)

		if pressure >= triggers.TextureEnterPressure {
			strongRun++
		} else {
			strongRun = 0
		}
		if pressure <= triggers.SpecularEnterPressure {
			quietRun++
		} else {
			quietRun = 0
		}

		switch state {
		case ProfileTexture:
			if pressure < 0.43 {
				state = ProfileBalanced
			}
		case ProfileSpecular:
			if pressure > 0.34 {
				state = ProfileBalanced
			}
		default:
			if strongRun >= 2 {
				state = ProfileTexture
			} else if quietRun >= 2 {
				state = ProfileSpecular
			}
		}

		baseKnee := previousKnee
		if i == 0 {
			baseKnee = rawKnee
		}
		knee := baseKnee + clamp(rawKnee-baseKnee, -opt.MaxSlew, opt.MaxSlew)
		cutScore := 0.0
		if i > 0 {
			cutScore = HistogramDistance(stats[i-1].LumaHistogram, stats[i].LumaHistogram)
		}
		dec := Decision{
			Frame: i, Profile: state, Knee: round(knee, 5), RawKnee: round(rawKnee, 5),
			Pressure: round(pressure, 4), Window: opt.Window, MaxSlew: opt.MaxSlew,
			SceneCut: cutScore >= 0.32, CutScore: round(cutScore, 4), Raw: stats[i],
			Stable: stable, Triggers: triggers,
		}
		dec.Reasons = explain(dec, strongRun, quietRun)
		decisions = append(decisions, dec)
		previousKnee = knee
	}
	return decisions
}

func medianFeatures(stats []FrameStats, center, window int) StableFeatures {
	radius := window / 2
	lo := maxInt(0, center-radius)
	hi := minInt(len(stats)-1, center+radius)
	get := func(f func(FrameStats) float64) float64 {
		values := make([]float64, 0, hi-lo+1)
		for i := lo; i <= hi; i++ {
			values = append(values, f(stats[i]))
		}
		sort.Float64s(values)
		return values[len(values)/2]
	}
	return StableFeatures{
		MeanLuma:              get(func(s FrameStats) float64 { return s.MeanLuma }),
		P995Max:               get(func(s FrameStats) float64 { return s.P995Max }),
		FractionOver1:         get(func(s FrameStats) float64 { return s.FractionOver1 }),
		FractionNearHighlight: get(func(s FrameStats) float64 { return s.FractionNearHighlight }),
		HighlightPixels:       get(func(s FrameStats) float64 { return s.HighlightPixels }),
		HighlightDetailRatio:  get(func(s FrameStats) float64 { return s.HighlightDetailRatio }),
	}
}

func highlightPressure(s StableFeatures) float64 {
	over := smoothstep(0.002, 0.030, s.FractionOver1)
	head := smoothstep(1.02, 1.80, s.P995Max)
	area := smoothstep(0.04, 0.28, s.FractionNearHighlight)
	detail := 0.0
	if s.HighlightPixels >= 0.025 {
		detail = smoothstep(1.05, 1.80, s.HighlightDetailRatio)
	}
	return clamp(0.30*over+0.25*head+0.20*area+0.25*detail, 0, 1)
}

func evaluateTriggers(s StableFeatures, pressure float64) TriggerConditions {
	t := TriggerConditions{
		OverRangePct: s.FractionOver1 * 100, OverRangeThreshold: 1.2,
		P995Max: s.P995Max, P995Threshold: 1.20,
		HighlightAreaPct: s.FractionNearHighlight * 100, AreaThresholdPct: 12,
		DetailRatio: s.HighlightDetailRatio, DetailThreshold: 1.25,
		Pressure: round(pressure, 4), TextureEnterPressure: 0.58, SpecularEnterPressure: 0.20,
	}
	t.OverRange = t.OverRangePct >= t.OverRangeThreshold
	t.Headroom = t.P995Max >= t.P995Threshold
	t.HighlightArea = t.HighlightAreaPct >= t.AreaThresholdPct
	t.HighlightTexture = s.HighlightPixels >= 0.025 && t.DetailRatio >= t.DetailThreshold
	return t
}

func explain(d Decision, strongRun, quietRun int) []string {
	t := d.Triggers
	reasons := []string{
		fmt.Sprintf("压力分=%.2f；已经过 %d 帧中位数稳定，knee 每帧限速 ≤%.3f", d.Pressure, d.Window, d.MaxSlew),
		fmt.Sprintf("超域像素=%.2f%%（触发阈值 ≥%.1f%%）：%t", t.OverRangePct, t.OverRangeThreshold, t.OverRange),
		fmt.Sprintf("P99.5 max RGB=%.3f（触发阈值 ≥%.2f）：%t", t.P995Max, t.P995Threshold, t.Headroom),
		fmt.Sprintf("高光面积=%.1f%%（触发阈值 ≥%.0f%%）：%t", t.HighlightAreaPct, t.AreaThresholdPct, t.HighlightArea),
		fmt.Sprintf("高光/全局梯度比=%.2f（面积≥2.5%% 时阈值 ≥%.2f）：%t", t.DetailRatio, t.DetailThreshold, t.HighlightTexture),
	}
	if d.Profile == ProfileTexture {
		reasons = append(reasons, fmt.Sprintf("场景档=%s：压力连续达到 ≥%.2f（当前连续计数=%d）或仍在 >0.43 的滞回区；knee=%.3f", d.Profile, t.TextureEnterPressure, strongRun, d.Knee))
	} else if d.Profile == ProfileSpecular {
		reasons = append(reasons, fmt.Sprintf("场景档=%s：压力连续低于 ≤%.2f（当前连续计数=%d）或仍在 <0.34 的滞回区；knee=%.3f", d.Profile, t.SpecularEnterPressure, quietRun, d.Knee))
	} else {
		reasons = append(reasons, fmt.Sprintf("场景档=%s；knee=%.3f", d.Profile, d.Knee))
	}
	if d.SceneCut {
		reasons = append(reasons, fmt.Sprintf("亮度直方图切换分数=%.2f ≥0.32；参数变化仍受每帧限速", d.CutScore))
	}
	return reasons
}

func smoothstep(edge0, edge1, x float64) float64 {
	if edge0 == edge1 {
		return 0
	}
	t := clamp((x-edge0)/(edge1-edge0), 0, 1)
	return t * t * (3 - 2*t)
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }

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

func round(v float64, digits int) float64 {
	p := math.Pow10(digits)
	return math.Round(v*p) / p
}
