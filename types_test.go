package hdrmeta

import (
	"testing"
	"time"
)

func TestTickDurationConversion(t *testing.T) {
	duration := 1500 * time.Millisecond
	ticks := TickFromDuration(duration)
	if ticks != 135_000 {
		t.Fatalf("ticks = %d, want 135000", ticks)
	}
	if got := ticks.Duration(); got != duration {
		t.Fatalf("duration = %s, want %s", got, duration)
	}
}
