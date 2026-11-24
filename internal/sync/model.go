// Package sync analyzes frame-correlated media positions across the
// capture, PTS, edit, render, and playback stages.
package sync

import (
	"fmt"
	"time"
)

// Stage is one observation point in the interview pipeline.
type Stage string

const (
	StageCapture  Stage = "capture"
	StagePTS      Stage = "pts"
	StageEdit     Stage = "edit"
	StageRender   Stage = "render"
	StagePlayback Stage = "playback"
)

// Stages is the canonical pipeline order.
var Stages = []Stage{
	StageCapture,
	StagePTS,
	StageEdit,
	StageRender,
	StagePlayback,
}

func ParseStage(value string) (Stage, error) {
	for _, stage := range Stages {
		if string(stage) == value {
			return stage, nil
		}
	}
	return "", fmt.Errorf("unknown stage %q", value)
}

// Record is one instrumentation event. Wide fields and long-format
// stream/position fields are both accepted.
type Record struct {
	Stage Stage `json:"stage"`

	// Frame is the stable/source frame identity, numbered from zero.
	Frame *int64 `json:"frame,omitempty"`
	// OutputFrame is the stage-specific output/encoder/display index.
	OutputFrame *int64 `json:"output_frame,omitempty"`

	// WallNS is the observation wall-clock time in Unix nanoseconds.
	WallNS int64 `json:"wall_ns,omitempty"`

	// Stream optionally identifies "video" or "audio" in long format.
	Stream string `json:"stream,omitempty"`

	// PositionNS is the generic media position in long format.
	PositionNS *int64 `json:"position_ns,omitempty"`

	// Wide-format positions allow one event to carry both streams.
	VideoPosNS *int64 `json:"video_pos_ns,omitempty"`
	AudioPosNS *int64 `json:"audio_pos_ns,omitempty"`

	Note string `json:"note,omitempty"`
}

// Point is a normalized record after long/wide field handling.
type Point struct {
	Stage       Stage
	Frame       int64
	HasFrame    bool
	OutputFrame *int64
	WallNS      int64

	PositionNS *int64
	VideoNS    *int64
	AudioNS    *int64
}

// Options controls frame timing and anomaly classification.
type Options struct {
	FPS           float64       `json:"fps"`
	FrameDuration time.Duration `json:"frame_duration_ns"`
	Tolerance     time.Duration `json:"tolerance_ns"`
	JumpThreshold time.Duration `json:"jump_threshold_ns"`
	DriftBudget   time.Duration `json:"drift_budget_ns"`
	MinSamples    int           `json:"min_samples"`
	Context       int           `json:"context"`
	FrameStride   int64         `json:"frame_stride,omitempty"`
}

// DefaultOptions assumes 25 fps, common for Chinese interview delivery.
// Callers should set FPS for 23.976/29.97/30/60 fps material.
func DefaultOptions() Options {
	return Options{
		FPS:           25,
		Tolerance:     2 * time.Millisecond,
		JumpThreshold: 20 * time.Millisecond,
		DriftBudget:   80 * time.Millisecond,
		MinSamples:    8,
		Context:       5,
	}
}

func (o Options) normalized() Options {
	if o.FPS <= 0 {
		o.FPS = 25
	}
	if o.FrameDuration <= 0 {
		o.FrameDuration = time.Duration(float64(time.Second) / o.FPS)
	}
	if o.Tolerance <= 0 {
		o.Tolerance = 2 * time.Millisecond
	}
	if o.JumpThreshold <= 0 {
		o.JumpThreshold = o.FrameDuration / 2
	}
	if o.DriftBudget <= 0 {
		o.DriftBudget = 2 * o.FrameDuration
	}
	if o.MinSamples < 4 {
		o.MinSamples = 4
	}
	if o.Context < 0 {
		o.Context = 0
	}
	return o
}

func absInt64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
