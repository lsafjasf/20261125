package cutluma

import (
	"encoding/json"
	"os"
)

// QuantileLevels are the per-frame distribution anchors exchanged by the
// tool. They cover shadow floor, midtones and highlight ceiling without
// requiring raw frames.
var QuantileLevels = []float64{0.005, 0.02, 0.10, 0.25, 0.50, 0.75, 0.90, 0.98, 0.995}

// FrameRow holds distribution quantiles for one frame.
// Out is measured after the segment color conversion; Src optionally holds
// the pre-conversion (source) quantiles, which enables direct attribution.
type FrameRow struct {
	Frame int       `json:"frame"`
	Out   []float64 `json:"out"`
	Src   []float64 `json:"src,omitempty"`
}

// SegmentJob describes one of the ten independently converted pieces.
type SegmentJob struct {
	Index  int            `json:"index"`
	Start  int            `json:"start"`
	End    int            `json:"end"` // exclusive
	Params *ConvertParams `json:"params,omitempty"`
}

// Job is the portable analysis input. It can be synthesized by this package
// or produced by a frame scanner in a real pipeline.
type Job struct {
	Name           string       `json:"name"`
	FrameCount     int          `json:"frameCount"`
	QuantileLevels []float64    `json:"quantileLevels"`
	Cuts           []int        `json:"cuts"`
	Frames         []FrameRow   `json:"frames"`
	Segments       []SegmentJob `json:"segments"`
}

// QIndex returns the column of a named quantile inside QuantileLevels.
func (j *Job) QIndex(q float64) int {
	for i, v := range j.QuantileLevels {
		if v == q {
			return i
		}
	}
	return -1
}

// SegmentOf maps a frame index to its segment index.
func (j *Job) SegmentOf(frame int) int {
	for i := len(j.Cuts) - 1; i >= 0; i-- {
		if frame >= j.Cuts[i] {
			return i + 1
		}
	}
	return 0
}

// SaveJSON writes indented JSON to path.
func SaveJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// LoadJobJSON reads a Job from path.
func LoadJobJSON(path string) (*Job, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, err
	}
	return &j, nil
}
