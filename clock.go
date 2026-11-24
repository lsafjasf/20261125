package mediaclock

import (
	"errors"
	"sync"
	"time"
)

var ErrNegativePosition = errors.New("mediaclock: position cannot be negative")

// ClockStats records state transitions. AutomaticGlobalSeeks is deliberately
// always zero: recovery re-anchors to audio samples and asks the video sink to
// wait or drop stale frames instead of repeatedly seeking the presentation.
type ClockStats struct {
	PlayCount              uint64
	PauseCount             uint64
	RateChanges            uint64
	SourceSwitches         uint64
	ExplicitSeeks          uint64
	HardwareRecalibrations uint64
	AutomaticGlobalSeeks   uint64
}

type clockAnchor struct {
	epoch          uint64
	hardwareFrames uint64
	sourcePTS      time.Duration
	timelinePTS    time.Duration
	speed          Speed
}

// MediaClock is the playback master clock. It is safe for concurrent use.
type MediaClock struct {
	mu     sync.Mutex
	hw     AudioHardware
	source Source
	speed  Speed

	running bool
	anchor  clockAnchor
	stats   ClockStats
}

func NewMediaClock(hw AudioHardware, source Source) (*MediaClock, error) {
	if hw == nil {
		return nil, errors.New("mediaclock: audio hardware clock is required")
	}
	normalized, err := source.withDefaults(hw.SampleRate())
	if err != nil {
		return nil, err
	}
	pos := hw.SamplePosition()
	speed := SpeedNormal
	return &MediaClock{
		hw:     hw,
		source: normalized,
		speed:  speed,
		anchor: clockAnchor{
			epoch:          pos.Epoch,
			hardwareFrames: pos.Frames,
			sourcePTS:      0,
			timelinePTS:    normalized.TimelineStart,
			speed:          speed,
		},
	}, nil
}

func (c *MediaClock) Source() Source {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.source
}

func (c *MediaClock) Stats() ClockStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *MediaClock) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

func (c *MediaClock) Speed() Speed {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.speed
}

// Play starts or resumes playback. The anchor is renewed at the current
// hardware sample, so time spent paused or in a throttled background timer is
// never counted as media time.
func (c *MediaClock) Play() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		return nil
	}
	pos := c.hw.SamplePosition()
	c.anchor.epoch = pos.Epoch
	c.anchor.hardwareFrames = pos.Frames
	c.anchor.speed = c.speed
	c.running = true
	c.stats.PlayCount++
	return nil
}

// Pause freezes the clock at the value derived from the last hardware sample.
func (c *MediaClock) Pause() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	sourcePTS, timelinePTS := c.timesLocked()
	pos := c.hw.SamplePosition()
	c.anchor = clockAnchor{
		epoch:          pos.Epoch,
		hardwareFrames: pos.Frames,
		sourcePTS:      sourcePTS,
		timelinePTS:    timelinePTS,
		speed:          c.speed,
	}
	c.running = false
	c.stats.PauseCount++
	return sourcePTS
}

func (c *MediaClock) SetRate(rate float64) error {
	speed, err := SpeedFromFloat(rate)
	if err != nil {
		return err
	}
	c.SetSpeed(speed)
	return nil
}

func (c *MediaClock) SetSpeed(speed Speed) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.running {
		sourcePTS, timelinePTS := c.timesLocked()
		pos := c.hw.SamplePosition()
		c.anchor = clockAnchor{
			epoch:          pos.Epoch,
			hardwareFrames: pos.Frames,
			sourcePTS:      sourcePTS,
			timelinePTS:    timelinePTS,
			speed:          speed,
		}
	} else {
		c.anchor.speed = speed
	}
	c.speed = speed
	c.stats.RateChanges++
}

// CurrentTime returns source-local media time.
func (c *MediaClock) CurrentTime() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	sourcePTS, _ := c.timesLocked()
	return sourcePTS
}

// Position is an alias for CurrentTime.
func (c *MediaClock) Position() time.Duration { return c.CurrentTime() }

// TimelineTime returns the position on the global composition timeline.
func (c *MediaClock) TimelineTime() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, timelinePTS := c.timesLocked()
	return timelinePTS
}

func (c *MediaClock) Times() (sourcePTS, timelinePTS time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.timesLocked()
}

// Recalibrate discards a stale software estimate and re-anchors to the paired
// hardware timestamp/sample position. It is not a seek and does not move media.
func (c *MediaClock) Recalibrate() (sourcePTS, timelinePTS time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sourcePTS, timelinePTS = c.timesLocked()
	pos := c.hw.SamplePosition()
	c.anchor.epoch = pos.Epoch
	c.anchor.hardwareFrames = pos.Frames
	c.anchor.sourcePTS = sourcePTS
	c.anchor.timelinePTS = timelinePTS
	c.anchor.speed = c.speed
	c.stats.HardwareRecalibrations++
	return sourcePTS, timelinePTS
}

// Foreground is the lifecycle hook for returning from background. It only
// recalibrates from audio hardware; callers must not perform a global seek.
func (c *MediaClock) Foreground() (sourcePTS, timelinePTS time.Duration) {
	return c.Recalibrate()
}

func (c *MediaClock) Background() {}

// Seek is an explicit user/program seek. Recovery from background drift must
// not call it periodically.
func (c *MediaClock) Seek(position time.Duration) error {
	if position < 0 {
		return ErrNegativePosition
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.source.Duration > 0 && position > c.source.Duration {
		position = c.source.Duration
	}
	pos := c.hw.SamplePosition()
	timelineOffset := c.anchor.timelinePTS - c.anchor.sourcePTS
	timeline := position + timelineOffset
	c.anchor = clockAnchor{
		epoch:          pos.Epoch,
		hardwareFrames: pos.Frames,
		sourcePTS:      position,
		timelinePTS:    timeline,
		speed:          c.speed,
	}
	c.stats.ExplicitSeeks++
	return nil
}

// SwitchSource changes the elementary stream while preserving both source-local
// and global timeline anchors. The old hardware counter remains monotonic.
func (c *MediaClock) SwitchSource(source Source, sourcePTS, timelinePTS time.Duration) error {
	normalized, err := source.withDefaults(c.hw.SampleRate())
	if err != nil {
		return err
	}
	if sourcePTS < 0 || timelinePTS < 0 {
		return ErrNegativePosition
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	pos := c.hw.SamplePosition()
	c.source = normalized
	c.anchor = clockAnchor{
		epoch:          pos.Epoch,
		hardwareFrames: pos.Frames,
		sourcePTS:      sourcePTS,
		timelinePTS:    timelinePTS,
		speed:          c.speed,
	}
	c.stats.SourceSwitches++
	return nil
}

type MediaSamplePosition struct {
	SourceID   string
	Frames     int64
	SampleRate int
	PTS        time.Duration
}

// MediaSamplePosition maps the current source PTS to an exact source sample.
func (c *MediaClock) MediaSamplePosition() MediaSamplePosition {
	c.mu.Lock()
	defer c.mu.Unlock()
	sourcePTS, _ := c.timesLocked()
	frames := DurationToFrames(sourcePTS, c.source.SampleRate)
	return MediaSamplePosition{
		SourceID:   c.source.ID,
		Frames:     int64(frames),
		SampleRate: c.source.SampleRate,
		PTS:        sourcePTS,
	}
}

func (c *MediaClock) timesLocked() (time.Duration, time.Duration) {
	anchor := c.anchor
	if !c.running {
		return anchor.sourcePTS, anchor.timelinePTS
	}

	pos := c.hw.SamplePosition()
	if pos.Epoch != anchor.epoch || pos.Frames < anchor.hardwareFrames {
		// A real device reset is a discontinuity. Preserve the last known media
		// position and establish a fresh sample anchor rather than using wall time.
		c.anchor.epoch = pos.Epoch
		c.anchor.hardwareFrames = pos.Frames
		c.anchor.speed = c.speed
		return anchor.sourcePTS, anchor.timelinePTS
	}

	sampleRate := pos.SampleRate
	if sampleRate <= 0 {
		sampleRate = c.hw.SampleRate()
	}
	deviceElapsed := FramesToDuration(pos.Frames-anchor.hardwareFrames, sampleRate)
	mediaElapsed := anchor.speed.MediaDuration(deviceElapsed)
	sourcePTS := anchor.sourcePTS + mediaElapsed
	timelinePTS := anchor.timelinePTS + mediaElapsed

	if c.source.Duration > 0 && sourcePTS > c.source.Duration {
		overflow := sourcePTS - c.source.Duration
		sourcePTS = c.source.Duration
		if timelinePTS > overflow {
			timelinePTS -= overflow
		}
	}
	return sourcePTS, timelinePTS
}
