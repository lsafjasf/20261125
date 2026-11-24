// Package timeline defines the canonical edit list: the single source of
// truth that every export format is derived from and verified against.
package timeline

import "encoding/json"
import (
	"fmt"
	"os"

	"cutcheck/internal/rational"
)

// TransitionType describes how a video event joins its predecessor.
type TransitionType string

const (
	TransitionCut      TransitionType = "cut"
	TransitionDissolve TransitionType = "dissolve"
)

// VideoEvent is one clip placement on a video track. Times are timeline
// frames at the edit list's frame rate; source times are frames in the
// source clip at the same rate.
type VideoEvent struct {
	ID            string         `json:"id"`
	Clip          string         `json:"clip"`
	Track         int            `json:"track"`
	RecordIn      int64          `json:"recordInFrames"`
	RecordOut     int64          `json:"recordOutFrames"`
	SourceIn      int64          `json:"sourceInFrames"`
	SourceOut     int64          `json:"sourceOutFrames"`
	Transition    TransitionType `json:"transition"`
	TransitionDur int64          `json:"transitionFrames,omitempty"`
	SpeedNum      int64          `json:"speedNum"` // retime ratio, 1/1 = normal
	SpeedDen      int64          `json:"speedDen"`
}

// AudioEvent is one clip placement on an audio track. Times are samples at
// the edit list's sample rate, giving sub-frame precision.
type AudioEvent struct {
	ID        string `json:"id"`
	Clip      string `json:"clip"`
	Track     int    `json:"track"`
	RecordIn  int64  `json:"recordInSamples"`
	RecordOut int64  `json:"recordOutSamples"`
	SourceIn  int64  `json:"sourceInSamples"`
	SourceOut int64  `json:"sourceOutSamples"`
	FadeIn    int64  `json:"fadeInSamples,omitempty"`
	FadeOut   int64  `json:"fadeOutSamples,omitempty"`
}

// EditList is the canonical edit decision list.
type EditList struct {
	Title      string       `json:"title"`
	FPSNum     int64        `json:"fpsNum"` // e.g. 30000/1001 = 29.97
	FPSDen     int64        `json:"fpsDen"`
	SampleRate int64        `json:"sampleRate"` // e.g. 48000
	Video      []VideoEvent `json:"video"`
	Audio      []AudioEvent `json:"audio"`
}

// Load reads an edit list from a JSON file.
func Load(path string) (*EditList, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var el EditList
	if err := json.Unmarshal(data, &el); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if el.FPSNum <= 0 || el.FPSDen <= 0 || el.SampleRate <= 0 {
		return nil, fmt.Errorf("parse %s: fps and sampleRate must be positive", path)
	}
	for i := range el.Video {
		if el.Video[i].SpeedNum == 0 {
			el.Video[i].SpeedNum = 1
		}
		if el.Video[i].SpeedDen == 0 {
			el.Video[i].SpeedDen = 1
		}
	}
	return &el, nil
}

// FPS returns the exact timeline frame rate.
func (el *EditList) FPS() rational.Rat { return rational.New(el.FPSNum, el.FPSDen) }

// FrameSeconds converts a timeline frame number to exact seconds.
func (el *EditList) FrameSeconds(frame int64) rational.Rat {
	return rational.New(frame*el.FPSDen, el.FPSNum)
}

// SecondsToFrame converts exact seconds to the nearest timeline frame.
func (el *EditList) SecondsToFrame(sec rational.Rat) int64 {
	return sec.Mul(el.FPS()).RoundInt64()
}

// SampleSeconds converts a sample index to exact seconds.
func (el *EditList) SampleSeconds(sample int64) rational.Rat {
	return rational.New(sample, el.SampleRate)
}

// SecondsToSample converts exact seconds to the nearest sample.
func (el *EditList) SecondsToSample(sec rational.Rat) int64 {
	return sec.Mul(rational.FromInt(el.SampleRate)).RoundInt64()
}

// Speed returns the exact retime ratio of a video event (1 = normal).
func (ev VideoEvent) Speed() rational.Rat { return rational.New(ev.SpeedNum, ev.SpeedDen) }

// IsRetime reports whether the event plays at non-normal speed.
func (ev VideoEvent) IsRetime() bool { return ev.SpeedNum != ev.SpeedDen }
