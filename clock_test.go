package mediaclock

import (
	"testing"
	"time"
)

func assertDurationClose(t *testing.T, got, want, tolerance time.Duration) {
	t.Helper()
	if diff := absDuration(got - want); diff > tolerance {
		t.Fatalf("duration = %s, want %s ± %s (diff %s)", got, want, tolerance, diff)
	}
}

func newTestClock(t *testing.T, rate int) (*AudioHardwareClock, *MediaClock) {
	t.Helper()
	hw := MustAudioHardwareClock(rate)
	clock, err := NewMediaClock(hw, Source{ID: "a", SampleRate: rate})
	if err != nil {
		t.Fatal(err)
	}
	if err := clock.Play(); err != nil {
		t.Fatal(err)
	}
	return hw, clock
}

func TestSpeedCommonRatesAreExact(t *testing.T) {
	cases := []struct {
		rate     float64
		num, den int64
		device   time.Duration
		media    time.Duration
	}{
		{1, 1, 1, time.Second, time.Second},
		{0.5, 1, 2, time.Second, 500 * time.Millisecond},
		{1.25, 5, 4, time.Second, 1250 * time.Millisecond},
		{1.5, 3, 2, time.Second, 1500 * time.Millisecond},
		{2, 2, 1, 500 * time.Millisecond, time.Second},
	}
	for _, tc := range cases {
		speed, err := SpeedFromFloat(tc.rate)
		if err != nil {
			t.Fatalf("rate %v: %v", tc.rate, err)
		}
		if speed.Num != tc.num || speed.Den != tc.den {
			t.Fatalf("rate %v = %d/%d, want %d/%d", tc.rate, speed.Num, speed.Den, tc.num, tc.den)
		}
		if got := speed.MediaDuration(tc.device); got != tc.media {
			t.Fatalf("rate %v mapped %s to %s, want %s", tc.rate, tc.device, got, tc.media)
		}
	}
}

func TestPauseAndBackgroundWallJumpDoNotAdvanceClock(t *testing.T) {
	hw, clock := newTestClock(t, 48_000)
	hw.AdvanceFrames(48_000)
	assertDurationClose(t, clock.CurrentTime(), time.Second, time.Microsecond)

	pausedAt := clock.Pause()
	hw.Pause()

	// The operating system may throttle timers and wall time may jump by hours.
	// The audio sample position is unchanged while the endpoint is paused.
	const backgroundWallJump = 6 * time.Hour
	clock.Background()
	if got := clock.CurrentTime(); got != pausedAt {
		t.Fatalf("paused clock changed to %s, pause position %s", got, pausedAt)
	}
	if hw.AdvanceFrames(48_000) != 0 {
		t.Fatal("paused hardware clock advanced")
	}

	clock.Foreground()
	hw.Start()
	if err := clock.Play(); err != nil {
		t.Fatal(err)
	}
	hw.AdvanceFrames(48_000)
	assertDurationClose(t, clock.CurrentTime(), 2*time.Second, time.Microsecond)
}

func TestRateChangesAnchorWithoutJumpOrAccumulation(t *testing.T) {
	hw, clock := newTestClock(t, 48_000)
	hw.AdvanceFrames(48_000)
	assertDurationClose(t, clock.CurrentTime(), time.Second, time.Microsecond)

	clock.SetSpeed(MustSpeed(2, 1))
	assertDurationClose(t, clock.CurrentTime(), time.Second, time.Microsecond)
	hw.AdvanceFrames(24_000)
	assertDurationClose(t, clock.CurrentTime(), 2*time.Second, time.Microsecond)

	clock.SetSpeed(MustSpeed(1, 2))
	hw.AdvanceFrames(48_000)
	assertDurationClose(t, clock.CurrentTime(), 5*time.Second/2, time.Microsecond)

	for i := 0; i < 100_000; i++ {
		hw.AdvanceFrames(1600)
	}
	// 160,000,000 frames / 48,000 = 3,333.333... device seconds at 0.5x.
	want := 5*time.Second/2 + time.Duration(100_000)*time.Second/60
	assertDurationClose(t, clock.CurrentTime(), want, time.Microsecond)
}

func TestSourceSwitchUsesNewSampleRateAndTimelineAnchor(t *testing.T) {
	hw, clock := newTestClock(t, 48_000)
	hw.AdvanceFrames(48_000)

	sourceB := Source{ID: "b", SampleRate: 44_100, Duration: 75 * time.Minute}
	if err := clock.SwitchSource(sourceB, 0, 105*time.Minute); err != nil {
		t.Fatal(err)
	}
	sourcePTS, timelinePTS := clock.Times()
	if sourcePTS != 0 || timelinePTS != 105*time.Minute {
		t.Fatalf("after switch = %s/%s", sourcePTS, timelinePTS)
	}

	hw.AdvanceFrames(48_000)
	assertDurationClose(t, clock.CurrentTime(), time.Second, time.Microsecond)
	assertDurationClose(t, clock.TimelineTime(), 105*time.Minute+time.Second, time.Microsecond)
	sample := clock.MediaSamplePosition()
	if sample.SourceID != "b" || sample.Frames != 44_100 || sample.SampleRate != 44_100 {
		t.Fatalf("sample = %+v", sample)
	}
	if clock.Stats().SourceSwitches != 1 {
		t.Fatalf("switches = %d", clock.Stats().SourceSwitches)
	}
}
