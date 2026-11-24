package mediaclock

import (
	"fmt"
	"math/big"
	"time"
)

const (
	AcceptanceFPS           = 30
	AcceptanceDeviceRate    = 48_000
	AcceptanceDuration      = 3 * time.Hour
	acceptanceSourceADur    = 105 * time.Minute
	acceptanceSourceBDur    = 75 * time.Minute
	acceptancePauseAt       = 12 * time.Minute
	acceptancePlayingBGAt   = 40 * time.Minute
	acceptanceSecondPause   = 112 * time.Minute
	acceptanceLongWallJump  = 6 * time.Hour
	acceptanceWallJumpTwo   = 2 * time.Hour
	acceptanceBackgroundRun = 10 * time.Second

	// AcceptanceDriftLimit is far below human perception and proves there is no
	// long-form clock accumulation over the synthetic three-hour timeline.
	AcceptanceDriftLimit = 2 * time.Millisecond
)

type AcceptanceReport struct {
	OK                      bool          `json:"ok"`
	TimelineDuration        time.Duration `json:"timeline_duration_ns"`
	FrameCount              int64         `json:"frame_count"`
	DeviceSampleRate        int           `json:"device_sample_rate"`
	PresentedFrames         int64         `json:"presented_frames"`
	DroppedFrames           int64         `json:"dropped_frames"`
	WaitedFrames            int64         `json:"waited_frames"`
	PauseCycles             uint64        `json:"pause_cycles"`
	BackgroundCycles        int           `json:"background_cycles"`
	SourceSwitches          uint64        `json:"source_switches"`
	RateChanges             uint64        `json:"rate_changes"`
	ExplicitSeeks           uint64        `json:"explicit_seeks"`
	HardwareRecalibrations  uint64        `json:"hardware_recalibrations"`
	AutomaticGlobalSeeks    uint64        `json:"automatic_global_seeks"`
	GlobalSeekCalls         int64         `json:"global_seek_calls"`
	MaxClockError           time.Duration `json:"max_clock_error_ns"`
	MaxPresentedDrift       time.Duration `json:"max_presented_drift_ns"`
	MaxWallJump             time.Duration `json:"max_wall_jump_ns"`
	FinalTimelinePosition   time.Duration `json:"final_timeline_position_ns"`
	FinalSourceID           string        `json:"final_source_id"`
	FinalSourceSampleFrames int64         `json:"final_source_sample_frames"`
	FinalHardwareFrames     uint64        `json:"final_hardware_frames"`
	FinalHardwareTime       time.Duration `json:"final_hardware_time_ns"`
	Failures                []string      `json:"failures,omitempty"`
}

type AcceptanceTimeline struct {
	Device  *AudioHardwareClock
	Clock   *MediaClock
	Video   *VideoScheduler
	Report  AcceptanceReport
	Pacer   *ratSamplePacer
	Wall    time.Duration
	MaxJump time.Duration
}

func NewAcceptanceTimeline() (*AcceptanceTimeline, error) {
	device := MustAudioHardwareClock(AcceptanceDeviceRate)
	segments := ThreeHourTimelineSegments()
	sourceA := segments[0].Source
	clock, err := NewMediaClock(device, sourceA)
	if err != nil {
		return nil, err
	}
	video := NewVideoScheduler(clock, SyncOptions{
		WaitThreshold: AcceptanceDriftLimit,
		DropThreshold: 5 * time.Millisecond,
	})
	frames := int64(AcceptanceDuration * AcceptanceFPS / time.Second)
	return &AcceptanceTimeline{
		Device: device,
		Clock:  clock,
		Video:  video,
		Pacer:  newRatSamplePacer(),
		Report: AcceptanceReport{
			TimelineDuration: AcceptanceDuration,
			FrameCount:       frames,
			DeviceSampleRate: AcceptanceDeviceRate,
		},
	}, nil
}

// ThreeHourTimelineSegments returns the deterministic source switches used by
// the acceptance timeline.
func ThreeHourTimelineSegments() []TimelineSegment {
	sourceA := Source{
		ID:            "feature-a/48k",
		SampleRate:    48_000,
		Duration:      acceptanceSourceADur,
		TimelineStart: 0,
	}
	sourceB := Source{
		ID:            "feature-b/44.1k",
		SampleRate:    44_100,
		Duration:      acceptanceSourceBDur,
		TimelineStart: acceptanceSourceADur,
	}
	return []TimelineSegment{
		{Source: sourceA, Start: 0, Duration: acceptanceSourceADur, SourceStart: 0},
		{Source: sourceB, Start: acceptanceSourceADur, Duration: acceptanceSourceBDur, SourceStart: 0},
	}
}

// RunThreeHourSyntheticTimeline is a descriptive alias for acceptance runners.
func RunThreeHourSyntheticTimeline() (AcceptanceReport, error) {
	return RunThreeHourAcceptance()
}

// RunThreeHourAcceptance executes a deterministic 3-hour composition timeline.
// It uses synthetic sample advancement rather than sleeping for three hours.
func RunThreeHourAcceptance() (AcceptanceReport, error) {
	tl, err := NewAcceptanceTimeline()
	if err != nil {
		return AcceptanceReport{}, err
	}
	if err := tl.Run(); err != nil {
		return tl.Report, err
	}
	tl.finishReport()
	return tl.Report, validateAcceptance(tl.Report)
}

func SyntheticFramePTS(index int64, fps int) time.Duration {
	return time.Duration(index * int64(time.Second) / int64(fps))
}

func acceptanceRateSchedule() map[int64]Speed {
	at := func(d time.Duration) int64 { return int64(d * AcceptanceFPS / time.Second) }
	return map[int64]Speed{
		at(10 * time.Minute):  MustSpeed(3, 2),
		at(30 * time.Minute):  MustSpeed(2, 1),
		at(50 * time.Minute):  MustSpeed(5, 4),
		at(70 * time.Minute):  MustSpeed(1, 2),
		at(90 * time.Minute):  SpeedNormal,
		at(120 * time.Minute): MustSpeed(3, 2),
		at(140 * time.Minute): MustSpeed(3, 4),
		at(160 * time.Minute): SpeedNormal,
	}
}

type ratSamplePacer struct {
	ideal *big.Rat
}

func newRatSamplePacer() *ratSamplePacer {
	return &ratSamplePacer{ideal: new(big.Rat)}
}

func (p *ratSamplePacer) addHardwareFrames(frames uint64) {
	p.ideal.Add(p.ideal, new(big.Rat).SetUint64(frames))
}

func (p *ratSamplePacer) step(device *AudioHardwareClock, fps int, speed Speed) uint64 {
	inc := new(big.Rat).SetFrac64(int64(device.SampleRate()), int64(fps))
	inc.Mul(inc, new(big.Rat).SetFrac64(speed.Den, speed.Num))
	p.ideal.Add(p.ideal, inc)

	target := roundRatUint64(p.ideal)
	current := device.SamplePosition().Frames
	if target <= current {
		return 0
	}
	advanced := target - current
	device.AdvanceFrames(advanced)
	return advanced
}

func roundRatUint64(r *big.Rat) uint64 {
	num := r.Num()
	den := r.Denom()
	q := new(big.Int).Quo(num, den)
	rem := new(big.Int).Rem(num, den)
	twiceRem := new(big.Int).Lsh(rem, 1)
	if twiceRem.Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Uint64()
}

func validateAcceptance(report AcceptanceReport) error {
	if len(report.Failures) != 0 {
		return fmt.Errorf("mediaclock: acceptance failed: %v", report.Failures)
	}
	if !report.OK {
		return fmt.Errorf("mediaclock: acceptance did not pass")
	}
	return nil
}
