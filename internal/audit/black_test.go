package audit

import (
	"fmt"
	"testing"
	"time"

	"cutmux/internal/media"
)

func TestFrameSidecarMatchesOnlyBlackRows(t *testing.T) {
	if black, ok := parseBlack("0"); !ok || black {
		t.Fatalf("parseBlack(0) = %t,%t", black, ok)
	}
	var rows [][]string
	rows = append(rows, []string{"pts_seconds", "black", "luma"})
	for i := 0; i < 12; i++ {
		black := "0"
		if i >= 10 {
			black = "1"
		}
		rows = append(rows, []string{
			formatSeconds(i * 40 * int(time.Millisecond)),
			black, "16",
		})
	}
	ps := make([]media.PresentedSample, 12)
	for i := range ps {
		ps[i].Start = int64(i * 40 * int(time.Millisecond))
		ps[i].End = ps[i].Start + int64(40*time.Millisecond)
	}
	ev, err := evidenceFromRows(rows, ps)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Summary.Matched != 2 {
		t.Fatalf("matched = %d, flags=%v, runs=%+v", ev.Summary.Matched, ev.flags, ev.Summary.Runs)
	}
	if ev.flags[0] || !ev.flags[10] || !ev.flags[11] {
		t.Fatalf("wrong flags: %v", ev.flags)
	}
}

func formatSeconds(ns int) string {
	return fmt.Sprintf("%.3f", float64(ns)/float64(time.Second))
}
