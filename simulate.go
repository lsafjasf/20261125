package mediaclock

import (
	"fmt"
	"time"
)

func (tl *AcceptanceTimeline) Run() error {
	if err := tl.Clock.Play(); err != nil {
		return err
	}

	rates := acceptanceRateSchedule()
	atFrame := func(d time.Duration) int64 { return int64(d * AcceptanceFPS / time.Second) }
	switchFrame := atFrame(acceptanceSourceADur)
	pauseFrame := atFrame(acceptancePauseAt)
	backgroundFrame := atFrame(acceptancePlayingBGAt)
	secondPauseFrame := atFrame(acceptanceSecondPause)

	for index := int64(0); index < tl.Report.FrameCount; index++ {
		pts := SyntheticFramePTS(index, AcceptanceFPS)

		if index == switchFrame {
			sourceB := ThreeHourTimelineSegments()[1].Source
			if err := tl.Clock.SwitchSource(sourceB, 0, acceptanceSourceADur); err != nil {
				return err
			}
		}

		decision := tl.Video.Schedule(VideoFrame{Index: index, PTS: pts})
		if decision.Action != ActionDrop {
			if errorAmount := absDuration(decision.Drift); errorAmount > tl.Report.MaxClockError {
				tl.Report.MaxClockError = errorAmount
			}
		}

		// Stale frames are dropped without advancing audio or media time.
		if decision.Action == ActionDrop {
			continue
		}

		if speed, ok := rates[index]; ok {
			tl.Clock.SetSpeed(speed)
		}

		skipNormalStep := false
		switch index {
		case pauseFrame:
			tl.pauseAndBackground(acceptanceLongWallJump, false)
		case secondPauseFrame:
			tl.pauseAndBackground(acceptanceWallJumpTwo, true)
		case backgroundFrame:
			tl.runInBackgroundWhilePlaying()
			skipNormalStep = true
		}

		if index+1 < tl.Report.FrameCount && !skipNormalStep {
			tl.Pacer.step(tl.Device, AcceptanceFPS, tl.Clock.Speed())
		}
	}

	// Advance through the final frame's duration to the three-hour endpoint.
	tl.Pacer.step(tl.Device, AcceptanceFPS, tl.Clock.Speed())
	return nil
}

func (tl *AcceptanceTimeline) pauseAndBackground(wallJump time.Duration, changeSpeed bool) {
	tl.Clock.Pause()
	tl.Device.Pause()

	tl.Wall += wallJump
	if wallJump > tl.MaxJump {
		tl.MaxJump = wallJump
	}
	tl.Clock.Background()

	if changeSpeed {
		tl.Clock.SetSpeed(MustSpeed(5, 4))
	}

	tl.Clock.Foreground()
	tl.Device.Start()
	tl.Clock.Play()
	tl.Report.BackgroundCycles++
}

func (tl *AcceptanceTimeline) runInBackgroundWhilePlaying() {
	tl.Wall += acceptanceBackgroundRun
	if acceptanceBackgroundRun > tl.MaxJump {
		tl.MaxJump = acceptanceBackgroundRun
	}

	played := tl.Device.Advance(acceptanceBackgroundRun)
	tl.Pacer.addHardwareFrames(played)
	tl.Clock.Background()
	tl.Clock.Foreground()
	_, _ = tl.Video.RecoverFromForeground()
	tl.Report.BackgroundCycles++
}

func (tl *AcceptanceTimeline) finishReport() {
	clockStats := tl.Clock.Stats()
	videoStats := tl.Video.Stats()
	sample := tl.Clock.MediaSamplePosition()

	tl.Report.PresentedFrames = videoStats.Presented
	tl.Report.DroppedFrames = videoStats.Dropped
	tl.Report.WaitedFrames = videoStats.Waited
	tl.Report.PauseCycles = clockStats.PauseCount
	tl.Report.SourceSwitches = clockStats.SourceSwitches
	tl.Report.RateChanges = clockStats.RateChanges
	tl.Report.ExplicitSeeks = clockStats.ExplicitSeeks
	tl.Report.HardwareRecalibrations = clockStats.HardwareRecalibrations
	tl.Report.AutomaticGlobalSeeks = clockStats.AutomaticGlobalSeeks
	tl.Report.GlobalSeekCalls = videoStats.GlobalSeekCalls
	tl.Report.MaxPresentedDrift = videoStats.MaxPresentedDrift
	tl.Report.MaxWallJump = tl.MaxJump
	tl.Report.FinalTimelinePosition = tl.Clock.TimelineTime()
	tl.Report.FinalSourceID = sample.SourceID
	tl.Report.FinalSourceSampleFrames = sample.Frames
	hardwarePosition := tl.Device.SamplePosition()
	tl.Report.FinalHardwareFrames = hardwarePosition.Frames
	tl.Report.FinalHardwareTime = hardwarePosition.Duration()

	report := &tl.Report
	report.Failures = nil
	addFailure := func(format string, args ...any) {
		report.Failures = append(report.Failures, fmt.Sprintf(format, args...))
	}

	if report.FrameCount != 324_000 {
		addFailure("frame count = %d, want 324000", report.FrameCount)
	}
	if report.PresentedFrames+report.DroppedFrames+report.WaitedFrames != report.FrameCount {
		addFailure("evaluated frames %d != %d", report.PresentedFrames+report.DroppedFrames+report.WaitedFrames, report.FrameCount)
	}
	if report.WaitedFrames != 0 {
		addFailure("%d frames unexpectedly waited", report.WaitedFrames)
	}
	if report.DroppedFrames == 0 {
		addFailure("background playback did not produce a stale-frame recovery case")
	}
	if report.SourceSwitches != 1 {
		addFailure("source switches = %d, want 1", report.SourceSwitches)
	}
	if report.PauseCycles != 2 {
		addFailure("pause cycles = %d, want 2", report.PauseCycles)
	}
	if report.BackgroundCycles != 3 {
		addFailure("background cycles = %d, want 3", report.BackgroundCycles)
	}
	if report.AutomaticGlobalSeeks != 0 || report.GlobalSeekCalls != 0 {
		addFailure("periodic/global seeks = clock:%d video:%d, want 0", report.AutomaticGlobalSeeks, report.GlobalSeekCalls)
	}
	if report.MaxClockError > AcceptanceDriftLimit {
		addFailure("max clock error %s > %s", report.MaxClockError, AcceptanceDriftLimit)
	}
	if report.MaxPresentedDrift > AcceptanceDriftLimit {
		addFailure("max presented A/V drift %s > %s", report.MaxPresentedDrift, AcceptanceDriftLimit)
	}
	if endError := absDuration(report.FinalTimelinePosition - AcceptanceDuration); endError > AcceptanceDriftLimit {
		addFailure("final timeline error %s > %s", endError, AcceptanceDriftLimit)
	}
	if report.FinalSourceID != "feature-b/44.1k" {
		addFailure("final source = %q", report.FinalSourceID)
	}

	report.OK = len(report.Failures) == 0
}
