package mediaclock

import (
	"errors"
	"sync"
	"time"
)

// SamplePosition is the authoritative playback position reported by an audio
// endpoint. Frames are frames already presented by the hardware/DAC, not
// compressed packets or bytes queued in userspace.
type SamplePosition struct {
	Frames     uint64
	SampleRate int
	Epoch      uint64
}

// Duration converts the hardware sample position to endpoint time.
func (p SamplePosition) Duration() time.Duration {
	return FramesToDuration(p.Frames, p.SampleRate)
}

// AudioHardware is implemented by platform audio adapters. The position must
// be sampled atomically with the hardware timestamp when the platform exposes
// both.
type AudioHardware interface {
	SampleRate() int
	SamplePosition() SamplePosition
}

// HardwareTimestamp pairs the audio hardware clock with its sample position.
type HardwareTimestamp struct {
	Time     time.Duration
	Samples  SamplePosition
	Latency  time.Duration
	WallJump time.Duration
}

// AudioHardwareClock is a deterministic audio endpoint clock. Production code
// should adapt CoreAudio, AAudio, WASAPI, or ALSA timestamp queries to
// AudioHardware; AdvanceFrames is used by the synthetic three-hour timeline.
type AudioHardwareClock struct {
	mu         sync.Mutex
	sampleRate int
	frames     uint64
	epoch      uint64
	running    bool
	latency    time.Duration
}

func NewAudioHardwareClock(sampleRate int) (*AudioHardwareClock, error) {
	if sampleRate <= 0 {
		return nil, errors.New("mediaclock: audio sample rate must be positive")
	}
	return &AudioHardwareClock{sampleRate: sampleRate, running: true}, nil
}

func MustAudioHardwareClock(sampleRate int) *AudioHardwareClock {
	clock, err := NewAudioHardwareClock(sampleRate)
	if err != nil {
		panic(err)
	}
	return clock
}

func (h *AudioHardwareClock) SampleRate() int { return h.sampleRate }

func (h *AudioHardwareClock) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.running
}

func (h *AudioHardwareClock) SetLatency(latency time.Duration) {
	h.mu.Lock()
	h.latency = latency
	h.mu.Unlock()
}

func (h *AudioHardwareClock) Start() {
	h.mu.Lock()
	h.running = true
	h.mu.Unlock()
}

// Pause freezes the hardware sample position. Buffered frames should be flushed
// by the platform adapter so a pause fence cannot leave audible tail running.
func (h *AudioHardwareClock) Pause() {
	h.mu.Lock()
	h.running = false
	h.mu.Unlock()
}

// ResetEpoch handles a genuine device reset. Ordinary source switches must not
// call this because the DAC frame counter can remain monotonic.
func (h *AudioHardwareClock) ResetEpoch() {
	h.mu.Lock()
	h.epoch++
	h.frames = 0
	h.running = false
	h.mu.Unlock()
}

func (h *AudioHardwareClock) SamplePosition() SamplePosition {
	h.mu.Lock()
	defer h.mu.Unlock()
	return SamplePosition{Frames: h.frames, SampleRate: h.sampleRate, Epoch: h.epoch}
}

func (h *AudioHardwareClock) Timestamp() HardwareTimestamp {
	h.mu.Lock()
	defer h.mu.Unlock()
	samples := SamplePosition{Frames: h.frames, SampleRate: h.sampleRate, Epoch: h.epoch}
	return HardwareTimestamp{Time: FramesToDuration(h.frames, h.sampleRate), Samples: samples, Latency: h.latency}
}

// Now is the conventional paired hardware-clock/sample-position query.
func (h *AudioHardwareClock) Now() HardwareTimestamp {
	return h.Timestamp()
}

// AdvanceFrames simulates DAC presentation. It has no effect while the endpoint
// is paused.
func (h *AudioHardwareClock) AdvanceFrames(frames uint64) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return 0
	}
	h.frames += frames
	return frames
}

// Advance is a convenience wrapper for tests and simulations.
func (h *AudioHardwareClock) Advance(deviceTime time.Duration) uint64 {
	frames := DurationToFrames(deviceTime, h.sampleRate)
	return h.AdvanceFrames(frames)
}

func FramesToDuration(frames uint64, sampleRate int) time.Duration {
	if sampleRate <= 0 {
		return 0
	}
	seconds := frames / uint64(sampleRate)
	remainder := frames % uint64(sampleRate)
	return time.Duration(seconds)*time.Second +
		time.Duration(remainder*uint64(time.Second)/uint64(sampleRate))
}

func DurationToFrames(d time.Duration, sampleRate int) uint64 {
	if d <= 0 || sampleRate <= 0 {
		return 0
	}
	return uint64(d.Nanoseconds()) * uint64(sampleRate) / uint64(time.Second)
}
