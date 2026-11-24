package cutluma

import (
	"path/filepath"
)

// TruthMatch pairs a scripted/expected cause with the diagnosed one.
type TruthMatch struct {
	CutIndex int    `json:"cutIndex"`
	Expected string `json:"expected"`
	Got      string `json:"got"`
	Matched  bool   `json:"matched"`
}

// Summary is the compact machine-readable verdict written to summary.json.
type Summary struct {
	JobName      string         `json:"jobName"`
	FrameCount   int            `json:"frameCount"`
	SegmentCount int            `json:"segmentCount"`
	CutCount     int            `json:"cutCount"`
	SigmaLuma    float64        `json:"sigmaLuma"`
	TargetParams ConvertParams  `json:"targetParams"`
	KindCounts   map[string]int `json:"kindCounts"`

	ConversionCuts  []int `json:"conversionCuts"`
	MixedCuts       []int `json:"mixedCuts"`
	NaturalCuts     []int `json:"naturalCuts"`
	PreexistingCuts []int `json:"preexistingCuts"`

	MaxConversionGapBefore     float64 `json:"maxConversionGapBefore"`
	MaxConversionGapAfter      float64 `json:"maxConversionGapAfter"`
	MeanConversionReductionPct float64 `json:"meanConversionReductionPct"`
	NaturalSlopeKeptMinPct     float64 `json:"naturalSlopeKeptMinPct"`

	Truth           []TruthMatch `json:"truth,omitempty"`
	AllTruthMatched bool         `json:"allTruthMatched"`

	Files map[string]string `json:"files"`
}

// WriteArtifacts dumps every deliverable into dir and returns the summary.
func WriteArtifacts(dir string, job *Job, rep *Report, plan *RepairPlan, truth []CutTruth) (*Summary, error) {
	files := map[string]string{}
	put := func(key, name string) string {
		p := filepath.Join(dir, name)
		files[key] = p
		return p
	}

	if err := SaveJSON(put("input", "input_job.json"), job); err != nil {
		return nil, err
	}
	if err := SaveJSON(put("report", "report.json"), rep); err != nil {
		return nil, err
	}
	if err := SaveJSON(put("repair", "repair_params.json"), plan); err != nil {
		return nil, err
	}
	if err := WriteSegmentCSV(put("segmentsCSV", "segment_report.csv"), rep); err != nil {
		return nil, err
	}
	if err := WriteCutCSV(put("cutsCSV", "cut_report.csv"), rep); err != nil {
		return nil, err
	}
	tl := BuildTimeline(job, plan)
	if err := WriteTimelineCSV(put("timelineCSV", "curve_timeline.csv"), tl); err != nil {
		return nil, err
	}
	cc := BuildCrossCutCurves(job, rep, plan)
	if err := WriteCrossCutCSV(put("crossCutCSV", "curves_cross_cut.csv"), cc); err != nil {
		return nil, err
	}
	if err := WriteSVG(put("svg", "curves.svg"), job, rep, plan); err != nil {
		return nil, err
	}

	sum := BuildSummary(job, rep, plan, truth)
	sum.Files = files
	if err := SaveJSON(put("summary", "summary.json"), sum); err != nil {
		return nil, err
	}
	if err := WriteMarkdownReport(put("markdown", "summary.md"), job, rep, plan, sum); err != nil {
		return nil, err
	}
	return sum, nil
}

// BuildSummary aggregates report/verification metrics.
func BuildSummary(job *Job, rep *Report, plan *RepairPlan, truth []CutTruth) *Summary {
	s := &Summary{
		JobName: job.Name, FrameCount: job.FrameCount,
		SegmentCount: len(job.Segments), CutCount: len(job.Cuts),
		SigmaLuma: rep.SigmaLuma, TargetParams: rep.TargetParams,
		KindCounts: map[string]int{}, AllTruthMatched: truth != nil,
		NaturalSlopeKeptMinPct: 100,
	}
	var reductions []float64
	for _, c := range rep.Cuts {
		s.KindCounts[c.Kind]++
		switch c.Kind {
		case KindConversion:
			s.ConversionCuts = append(s.ConversionCuts, c.CutIndex)
		case KindMixed:
			s.MixedCuts = append(s.MixedCuts, c.CutIndex)
		case KindNatural:
			s.NaturalCuts = append(s.NaturalCuts, c.CutIndex)
		case KindPreexisting:
			s.PreexistingCuts = append(s.PreexistingCuts, c.CutIndex)
		}
	}
	for _, v := range plan.Verification {
		switch v.Kind {
		case KindConversion, KindMixed:
			if v.AbsBefore > s.MaxConversionGapBefore {
				s.MaxConversionGapBefore = v.AbsBefore
			}
			if v.AbsAfter > s.MaxConversionGapAfter {
				s.MaxConversionGapAfter = v.AbsAfter
			}
			reductions = append(reductions, v.ReductionPct)
		case KindNatural:
			if v.SlopeKeptPct > 0 && v.SlopeKeptPct < s.NaturalSlopeKeptMinPct {
				s.NaturalSlopeKeptMinPct = v.SlopeKeptPct
			}
		}
	}
	if len(reductions) > 0 {
		s.MeanConversionReductionPct = meanFloat(reductions)
	}
	if truth != nil {
		for _, t := range truth {
			gm := TruthMatch{CutIndex: t.CutIndex, Expected: t.Kind}
			if t.CutIndex < len(rep.Cuts) {
				gm.Got = rep.Cuts[t.CutIndex].Kind
				gm.Matched = gm.Got == gm.Expected
			}
			s.Truth = append(s.Truth, gm)
			if !gm.Matched {
				s.AllTruthMatched = false
			}
		}
	}
	return s
}
