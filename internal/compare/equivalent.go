package compare

import (
	"cutcheck/internal/rational"
	"cutcheck/internal/timeline"
	"fmt"
)

// Tolerance is the maximum allowed timing error for one (format, layer),
// expressed in the layer's native unit (frames or samples).
type Tolerance struct {
	Format   string
	Layer    string
	MaxUnits float64
}

// DefaultTolerances derives sensible tolerances: video must be frame-exact,
// FCPXML audio sample-exact, and CMX audio within half a frame (CMX
// quantizes audio to frame boundaries).
func DefaultTolerances(el *timeline.EditList) []Tolerance {
	halfFrameSamples := el.FrameSeconds(1).
		Mul(rational.New(el.SampleRate, 2)).Float64()
	return []Tolerance{
		{Format: "cmx", Layer: "video", MaxUnits: 0.5},
		{Format: "cmx", Layer: "audio", MaxUnits: halfFrameSamples},
		{Format: "fcpxml", Layer: "video", MaxUnits: 0.5},
		{Format: "fcpxml", Layer: "audio", MaxUnits: 0.5},
	}
}

// Failures returns one line per equivalence violation (empty = equivalent).
func Failures(el *timeline.EditList, deltas []EventDelta, missing []string, tols []Tolerance) []string {
	tol := map[string]float64{}
	for _, t := range tols {
		tol[t.Format+"/"+t.Layer] = t.MaxUnits
	}
	var out []string
	out = append(out, missing...)
	for _, d := range deltas {
		scale, unit := unitScale(el, d.Layer)
		max := tol[d.Format+"/"+d.Layer]
		in := d.RecordIn.Mul(scale).Float64()
		o := d.RecordOut.Mul(scale).Float64()
		if abs(in) > max || abs(o) > max {
			out = append(out, fmt.Sprintf(
				"%s/%s event %s: recordIn %+0.1f %s, recordOut %+0.1f %s (tolerance %.1f)",
				d.Format, d.Layer, d.EventID, in, unit, o, unit, max))
		}
	}
	return out
}
