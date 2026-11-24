package mediaclock

import (
	"sync"
	"time"
)

type FrameAction int

const (
	ActionPresent FrameAction = iota
	ActionWait
	ActionDrop
)

func (a FrameAction) String() string {
	switch a {
	case ActionPresent:
		return "present"
	case ActionWait:
		return "wait"
	case ActionDrop:
		return "drop"
	default:
		return "unknown"
	}
}

// VideoFrame uses PTS as the global timeline presentation timestamp.
type VideoFrame struct {
	Index int64
	PTS   time.Duration
}

type FrameDecision struct {
	Frame  VideoFrame
	Action FrameAction
	Clock  time.Duration
	Drift  time.Duration
	Sample MediaSamplePosition
}

type VideoStats struct {
	Evaluated         int64
	Presented         int64
	Waited            int64
	Dropped           int64
	MaxAbsDrift       time.Duration
	MaxPresentedDrift time.Duration
	LastDrift         time.Duration
	LocalResyncs      int64
	GlobalSeekCalls   int64
}

type SyncOptions struct {
	// WaitThreshold asks the renderer to wait when a frame is early by more.
	WaitThreshold time.Duration
	// DropThreshold drops stale frames when video is behind audio by more.
	DropThreshold time.Duration
}

func DefaultSyncOptions() SyncOptions {
	return SyncOptions{WaitThreshold: 8 * time.Millisecond, DropThreshold: 45 * time.Millisecond}
}

// VideoScheduler aligns video frames to the audio-hardware-backed MediaClock.
type VideoScheduler struct {
	mu    sync.Mutex
	clock *MediaClock
	opts  SyncOptions
	stats VideoStats
}

func NewVideoScheduler(clock *MediaClock, opts SyncOptions) *VideoScheduler {
	if opts.WaitThreshold <= 0 {
		opts.WaitThreshold = DefaultSyncOptions().WaitThreshold
	}
	if opts.DropThreshold <= 0 {
		opts.DropThreshold = DefaultSyncOptions().DropThreshold
	}
	return &VideoScheduler{clock: clock, opts: opts}
}

func (v *VideoScheduler) Stats() VideoStats {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.stats
}

// Schedule evaluates a frame at the current media clock. A large residual is
// handled by dropping through the backlog. It never invokes a global seek.
func (v *VideoScheduler) Schedule(frame VideoFrame) FrameDecision {
	_, timeline := v.clock.Times()
	drift := frame.PTS - timeline
	action := ActionPresent

	switch {
	case drift > v.opts.WaitThreshold:
		action = ActionWait
	case -drift > v.opts.DropThreshold:
		action = ActionDrop
	}

	v.mu.Lock()
	v.stats.Evaluated++
	v.stats.LastDrift = drift
	if absDuration(drift) > v.stats.MaxAbsDrift {
		v.stats.MaxAbsDrift = absDuration(drift)
	}
	switch action {
	case ActionPresent:
		v.stats.Presented++
		if absDuration(drift) > v.stats.MaxPresentedDrift {
			v.stats.MaxPresentedDrift = absDuration(drift)
		}
	case ActionWait:
		v.stats.Waited++
	case ActionDrop:
		v.stats.Dropped++
	}
	v.mu.Unlock()

	return FrameDecision{
		Frame:  frame,
		Action: action,
		Clock:  timeline,
		Drift:  drift,
		Sample: v.clock.MediaSamplePosition(),
	}
}

// RecoverFromForeground samples audio hardware and clears a stale video
// backlog marker. The returned cutoff lets the renderer drop frames before it.
func (v *VideoScheduler) RecoverFromForeground() (cutoff time.Duration, decision FrameDecision) {
	_, timeline := v.clock.Foreground()
	v.mu.Lock()
	v.stats.LocalResyncs++
	v.mu.Unlock()
	return timeline - v.opts.DropThreshold, FrameDecision{Action: ActionWait, Clock: timeline}
}

// GlobalSeek exists only to make an explicit repair boundary visible. The
// acceptance timeline asserts that periodic correction never calls it.
func (v *VideoScheduler) GlobalSeek(position time.Duration, seek func(time.Duration) error) error {
	if err := seek(position); err != nil {
		return err
	}
	v.mu.Lock()
	v.stats.GlobalSeekCalls++
	v.mu.Unlock()
	return nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
