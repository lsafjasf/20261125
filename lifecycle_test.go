package mediaclock

import (
	"sync"
	"testing"
	"time"
)

func TestExplicitSeekReanchorsSampleClock(t *testing.T) {
	hw, clock := newTestClock(t, 48_000)
	hw.AdvanceFrames(48_000)

	if err := clock.Seek(30 * time.Second); err != nil {
		t.Fatal(err)
	}
	if clock.Stats().ExplicitSeeks != 1 {
		t.Fatal("explicit seek was not recorded")
	}
	assertDurationClose(t, clock.CurrentTime(), 30*time.Second, time.Microsecond)

	hw.AdvanceFrames(48_000)
	assertDurationClose(t, clock.CurrentTime(), 31*time.Second, time.Microsecond)
	if clock.Stats().AutomaticGlobalSeeks != 0 {
		t.Fatal("explicit seek changed automatic repair counter")
	}
}

func TestSwitchSourceWhilePaused(t *testing.T) {
	_, clock := newTestClock(t, 48_000)
	clock.Pause()

	err := clock.SwitchSource(Source{ID: "paused-b", SampleRate: 44_100}, 2*time.Second, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if clock.IsRunning() {
		t.Fatal("source switch resumed a paused clock")
	}
	sourcePTS, timelinePTS := clock.Times()
	if sourcePTS != 2*time.Second || timelinePTS != 20*time.Second {
		t.Fatalf("positions = %s/%s", sourcePTS, timelinePTS)
	}
}

func TestMediaClockConcurrentAccess(t *testing.T) {
	hw, clock := newTestClock(t, 48_000)
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = clock.Times()
					_ = clock.CurrentTime()
					_ = clock.MediaSamplePosition()
					_ = clock.Stats()
				}
			}
		}()
	}

	speeds := []Speed{SpeedNormal, MustSpeed(3, 2), MustSpeed(1, 2), MustSpeed(5, 4)}
	for i := 0; i < 20_000; i++ {
		hw.AdvanceFrames(1600)
		if i%128 == 0 {
			clock.SetSpeed(speeds[(i/128)%len(speeds)])
		}
	}
	close(stop)
	wg.Wait()
}
