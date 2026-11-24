package mediaclock

import (
	"errors"
	"time"
)

// Source identifies one switchable media elementary stream. SampleRate is the
// source audio rate; the hardware endpoint may run at a different rate.
type Source struct {
	ID            string
	SampleRate    int
	Duration      time.Duration
	TimelineStart time.Duration
}

func (s Source) withDefaults(hardwareRate int) (Source, error) {
	if hardwareRate <= 0 {
		return Source{}, errors.New("mediaclock: invalid hardware sample rate")
	}
	if s.ID == "" {
		s.ID = "default"
	}
	if s.SampleRate == 0 {
		s.SampleRate = hardwareRate
	}
	if s.SampleRate <= 0 {
		return Source{}, errors.New("mediaclock: source sample rate must be positive")
	}
	if s.Duration < 0 || s.TimelineStart < 0 {
		return Source{}, errors.New("mediaclock: source time range is negative")
	}
	return s, nil
}

// TimelineSegment is one source placed on a global composition timeline.
type TimelineSegment struct {
	Source      Source
	Start       time.Duration
	Duration    time.Duration
	SourceStart time.Duration
}

func (seg TimelineSegment) contains(t time.Duration) bool {
	if t < seg.Start {
		return false
	}
	if seg.Duration == 0 {
		return t == seg.Start
	}
	return t < seg.Start+seg.Duration
}

// SegmentAt locates the source that owns global timeline position t.
func SegmentAt(segments []TimelineSegment, t time.Duration) (TimelineSegment, time.Duration, bool) {
	for _, seg := range segments {
		if seg.contains(t) {
			return seg, seg.SourceStart + (t - seg.Start), true
		}
	}
	return TimelineSegment{}, 0, false
}
