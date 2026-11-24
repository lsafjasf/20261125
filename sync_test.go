package mediaclock

import (
	"testing"
	"time"
)

func TestHardwareSamplePositionIsAuthoritative(t *testing.T) {
	hw := MustAudioHardwareClock(48_000)
	if advanced := hw.AdvanceFrames(48_000); advanced != 48_000 {
		t.Fatalf("advanced = %d", advanced)
	}
	pos := hw.SamplePosition()
	if pos.Frames != 48_000 || pos.SampleRate != 48_000 {
		t.Fatalf("position = %+v", pos)
	}
	if pos.Duration() != time.Second {
		t.Fatalf("sample duration = %s", pos.Duration())
	}

	hw.Pause()
	if hw.AdvanceFrames(100) != 0 {
		t.Fatal("paused endpoint reported rendered frames")
	}
	if hw.SamplePosition().Frames != 48_000 {
		t.Fatal("sample position moved while paused")
	}
}

func TestBackgroundPlayingDropsStaleFramesWithoutGlobalSeek(t *testing.T) {
	hw, clock := newTestClock(t, 48_000)
	scheduler := NewVideoScheduler(clock, SyncOptions{
		WaitThreshold: time.Millisecond,
		DropThreshold: 20 * time.Millisecond,
	})

	if decision := scheduler.Schedule(VideoFrame{Index: 0, PTS: 0}); decision.Action != ActionPresent {
		t.Fatalf("initial action = %s", decision.Action)
	}

	// Audio continues while the page is backgrounded. Ten device seconds are
	// authoritative even if timer callbacks are throttled.
	hw.Advance(10 * time.Second)
	clock.Foreground()

	stale := scheduler.Schedule(VideoFrame{Index: 30, PTS: time.Second})
	if stale.Action != ActionDrop {
		t.Fatalf("stale frame action = %s drift=%s", stale.Action, stale.Drift)
	}

	current := scheduler.Schedule(VideoFrame{Index: 300, PTS: 10 * time.Second})
	if current.Action != ActionPresent {
		t.Fatalf("current frame action = %s drift=%s", current.Action, current.Drift)
	}
	assertDurationClose(t, current.Drift, 0, time.Millisecond)

	stats := scheduler.Stats()
	if stats.GlobalSeekCalls != 0 {
		t.Fatalf("global seek calls = %d", stats.GlobalSeekCalls)
	}
	if clock.Stats().AutomaticGlobalSeeks != 0 {
		t.Fatal("automatic global seeks must remain zero")
	}
}

func TestThreeHourSyntheticAcceptance(t *testing.T) {
	report, err := RunThreeHourAcceptance()
	if err != nil {
		t.Fatalf("acceptance report: %+v: %v", report, err)
	}
	if !report.OK {
		t.Fatalf("acceptance failed: %+v", report.Failures)
	}
	if report.FrameCount != 324_000 {
		t.Fatalf("frame count = %d", report.FrameCount)
	}
	if report.MaxPresentedDrift > AcceptanceDriftLimit {
		t.Fatalf("presented drift %s > %s", report.MaxPresentedDrift, AcceptanceDriftLimit)
	}
	if report.MaxClockError > AcceptanceDriftLimit {
		t.Fatalf("clock error %s > %s", report.MaxClockError, AcceptanceDriftLimit)
	}
	if report.AutomaticGlobalSeeks != 0 || report.GlobalSeekCalls != 0 {
		t.Fatal("acceptance used a global seek repair")
	}
}
