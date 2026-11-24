package vdetect

import "testing"

func TestRollingSeamClassifiedAndTracked(t *testing.T) {
	base := testScene(640, 360, 8)
	frames := make([]*Frame, 10)
	for i := range frames {
		frames[i] = testTearFrame(base, 45+i*27, 18)
	}
	rep := analyzeFrames(frames)
	if rep.Summary.RollingFrames < 8 {
		t.Fatalf("rolling frames = %d, report: %+v", rep.Summary.RollingFrames, rep.Summary)
	}
	tr := findTrack(rep, "capture_rolling")
	if tr == nil {
		t.Fatalf("missing rolling track: %+v", rep.Tracks)
	}
	if tr.MonotonicRatio < 0.85 || absFloat(tr.VelocityYPx-27) > 6 {
		t.Fatalf("bad rolling motion: mono=%.2f velocity=%.2f", tr.MonotonicRatio, tr.VelocityYPx)
	}
	if tr.Continuity < 0.9 {
		t.Fatalf("rolling continuity = %.2f", tr.Continuity)
	}
}

func TestFieldOrderClassified(t *testing.T) {
	base := testScene(640, 360, 11)
	frames := make([]*Frame, 6)
	for i := range frames {
		frames[i] = testFieldFrame(base, 4)
	}
	rep := analyzeFrames(frames)
	if rep.Summary.FieldOrderFrames != 6 {
		t.Fatalf("field frames = %d, want 6", rep.Summary.FieldOrderFrames)
	}
	tr := findTrack(rep, "field_order_error")
	if tr == nil || tr.Continuity < 0.95 {
		t.Fatalf("bad field track: %+v", rep.Tracks)
	}
}

func TestBlockDefectPersists(t *testing.T) {
	base := testScene(640, 360, 23)
	corrupt := testBlockFrame(base, 128, 96, 16)
	rep := analyzeFrames([]*Frame{cloneTestFrame(base), corrupt, cloneTestFrame(corrupt)})
	if rep.Summary.BlockFrames != 2 {
		t.Fatalf("block frames = %d, frames=%+v", rep.Summary.BlockFrames, rep.Frames)
	}
	tr := findTrack(rep, "macroblock_corruption")
	if tr == nil || tr.FrameCount < 2 || tr.Continuity < 0.9 {
		t.Fatalf("bad block track: %+v", rep.Tracks)
	}
	p := tr.Positions[0]
	if absInt(p.X-128) > 4 || absInt(p.Y-96) > 4 {
		t.Fatalf("block position = (%d,%d), want (128,96)", p.X, p.Y)
	}
}

func TestBrighteningHasNoStructuralFalsePositives(t *testing.T) {
	base := testScene(640, 360, 31)
	frames := make([]*Frame, 18)
	for i := range frames {
		frames[i] = testFadeFrame(base, i*4)
	}
	rep := analyzeFrames(frames)
	if rep.Summary.TearFrames != 0 || rep.Summary.BlockFrames != 0 || rep.Summary.FieldOrderFrames != 0 {
		t.Fatalf("fade produced structural findings: %+v", rep.Summary)
	}
	for _, fr := range rep.Frames {
		if fr.Guards.MotionCoverage > 0.18 {
			t.Fatalf("frame %d fade motion coverage too large: %.2f", fr.Index, fr.Guards.MotionCoverage)
		}
	}
}

func TestMovingForegroundHasNoTearOrBlockFalsePositives(t *testing.T) {
	base := testScene(640, 360, 41)
	frames := make([]*Frame, 12)
	for i := range frames {
		frames[i] = testMovingFrame(base, 20+i*45)
	}
	rep := analyzeFrames(frames)
	if rep.Summary.TearFrames != 0 || rep.Summary.BlockFrames != 0 || rep.Summary.FieldOrderFrames != 0 {
		for _, fr := range rep.Frames {
			for _, b := range fr.Blocks {
				t.Logf("frame %d block %+v", fr.Index, b)
			}
		}
		for _, tr := range rep.Tracks {
			t.Logf("track %+v", tr)
		}
		t.Fatalf("moving foreground produced structural findings: %+v", rep.Summary)
	}
	for _, fr := range rep.Frames {
		if fr.Guards.MotionCoverage > 0.20 {
			t.Fatalf("frame %d motion coverage = %.2f, too broad", fr.Index, fr.Guards.MotionCoverage)
		}
		for _, label := range fr.Labels {
			if label == "tear" || label == "macroblock_corruption" || label == "field_order_error" {
				t.Fatalf("frame %d has structural label %q", fr.Index, label)
			}
		}
	}
}

func TestSparseIntermittentTearTrack(t *testing.T) {
	base := testScene(640, 360, 51)
	clean := cloneTestFrame(base)
	bad := testTearFrame(base, 210, -22)
	frames := []*Frame{clean, bad, cloneTestFrame(base), cloneTestFrame(base), bad, cloneTestFrame(base)}
	rep := analyzeFrames(frames)
	if rep.Summary.TearFrames != 2 {
		t.Fatalf("tear frames = %d, want 2", rep.Summary.TearFrames)
	}
	var tearTracks int
	for _, tr := range rep.Tracks {
		if tr.Kind == "tear" {
			tearTracks++
			if tr.FrameCount != 2 {
				t.Fatalf("intermittent observations not retained: %+v", tr)
			}
		}
	}
	if tearTracks != 1 {
		t.Fatalf("intermittent tear tracks = %d, want 1", tearTracks)
	}
}
