package compare

import (
	"cutcheck/internal/rational"
	"cutcheck/internal/timeline"
	"fmt"
	"sort"
)

// Pattern classifies a set of per-event timing errors.
type Pattern string

const (
	PatternNone      Pattern = "match"
	PatternConstant  Pattern = "constant-offset"
	PatternDrift     Pattern = "cumulative-drift"
	PatternScattered Pattern = "scattered"
)

// Diagnosis localizes discrepancies to one (format, layer) pair.
type Diagnosis struct {
	Format  string
	Layer   string
	Pattern Pattern
	Unit    string  // "frames" or "samples"
	MaxAbs  float64 // largest |delta| in Unit
	Detail  string  // root-cause hint
}

// unitScale converts a seconds delta to the layer's native unit.
func unitScale(el *timeline.EditList, layer string) (rational.Rat, string) {
	if layer == "video" {
		return el.FPS(), "frames"
	}
	return rational.FromInt(el.SampleRate), "samples"
}

// canonicalPos returns the canonical record-in position (seconds) of an
// event, used as the x-axis for drift detection.
func canonicalPos(el *timeline.EditList, id, layer string) float64 {
	if layer == "video" {
		for _, ev := range el.Video {
			if ev.ID == id {
				return el.FrameSeconds(ev.RecordIn).Float64()
			}
		}
		return 0
	}
	for _, ev := range el.Audio {
		if ev.ID == id {
			return el.SampleSeconds(ev.RecordIn).Float64()
		}
	}
	return 0
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Diagnose groups deltas by (format, layer) and classifies the error
// pattern of each group. Groups whose largest error is within the given
// tolerance are reported as PatternNone.
func Diagnose(el *timeline.EditList, deltas []EventDelta, tols []Tolerance) []Diagnosis {
	tol := map[string]float64{}
	for _, t := range tols {
		tol[t.Format+"/"+t.Layer] = t.MaxUnits
	}
	type key struct{ format, layer string }
	groups := map[key][]EventDelta{}
	var order []key
	for _, d := range deltas {
		k := key{d.Format, d.Layer}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], d)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].format != order[j].format {
			return order[i].format < order[j].format
		}
		return order[i].layer < order[j].layer
	})

	var out []Diagnosis
	for _, k := range order {
		ds := groups[k]
		scale, unit := unitScale(el, k.layer)
		units := make([]float64, len(ds))
		pos := make([]float64, len(ds))
		maxAbs := 0.0
		for i, d := range ds {
			u := d.RecordIn.Mul(scale).Float64()
			units[i] = u
			if a := abs(u); a > maxAbs {
				maxAbs = a
			}
			pos[i] = canonicalPos(el, d.EventID, k.layer)
		}
		dg := Diagnosis{Format: k.format, Layer: k.layer, Unit: unit, MaxAbs: maxAbs}
		switch {
		case maxAbs <= tol[k.format+"/"+k.layer]:
			dg.Pattern = PatternNone
		case isConstant(units):
			dg.Pattern = PatternConstant
			dg.Detail = constantHint(k.format, k.layer, units[0], unit)
		case isDrift(units, pos):
			dg.Pattern = PatternDrift
			dg.Detail = driftHint(k.format, k.layer)
		default:
			dg.Pattern = PatternScattered
			dg.Detail = "no uniform pattern; inspect events individually"
		}
		out = append(out, dg)
	}
	return out
}

// isConstant reports whether all values are equal within half a unit.
func isConstant(v []float64) bool {
	if len(v) == 0 {
		return false
	}
	for _, x := range v[1:] {
		if abs(x-v[0]) > 0.5 {
			return false
		}
	}
	return true
}

// isDrift reports whether the error grows linearly with timeline position
// (least-squares fit through the origin with small residuals).
func isDrift(units, pos []float64) bool {
	var sp, pp float64
	for i := range units {
		sp += units[i] * pos[i]
		pp += pos[i] * pos[i]
	}
	if pp == 0 {
		return false
	}
	k := sp / pp
	if abs(k) < 1e-6 {
		return false
	}
	for i := range units {
		if abs(units[i]-k*pos[i]) > 0.75 {
			return false
		}
	}
	return true
}

func constantHint(format, layer string, delta float64, unit string) string {
	if layer == "audio" {
		return fmt.Sprintf("all audio events shifted by %+.0f %s; typical of uncompensated codec priming/latency",
			delta, unit)
	}
	return fmt.Sprintf("all video events shifted by %+.0f %s; check start-timecode offset", delta, unit)
}

func driftHint(format, layer string) string {
	if format == "cmx" {
		return "error grows with timeline position; CMX timecodes were written at a wrong frame rate (e.g. 30/1 vs 30000/1001)"
	}
	if layer == "audio" {
		return "error grows with timeline position; check sample-rate mismatch"
	}
	return "error grows with timeline position; check frame-rate mismatch"
}
