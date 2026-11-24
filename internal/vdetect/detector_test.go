package vdetect

import (
	"math/rand"
	"testing"
)

func testScene(w, h int, seed int64) *Frame {
	r := rand.New(rand.NewSource(seed))
	f := &Frame{W: w, H: h, Y: make([]byte, w*h), R: make([]byte, w*h), G: make([]byte, w*h), B: make([]byte, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 40 + 80*x/w + 50*y/h
			f.Y[y*w+x] = byte(clampInt(v, 0, 255))
		}
	}
	for i := 0; i < 45; i++ {
		x0, y0 := r.Intn(w), r.Intn(h)
		rw, rh := 10+r.Intn(w/5), 8+r.Intn(h/4)
		col := byte(r.Intn(255))
		for y := y0; y < minInt(h, y0+rh); y++ {
			for x := x0; x < minInt(w, x0+rw); x++ {
				idx := y*w + x
				f.Y[idx] = col
			}
		}
	}
	for i := 0; i < 18; i++ {
		x := r.Intn(w)
		col := byte(r.Intn(255))
		thick := 1 + r.Intn(4)
		for tx := -thick; tx <= thick; tx++ {
			xx := x + tx
			if xx < 0 || xx >= w {
				continue
			}
			for y := 0; y < h; y++ {
				f.Y[y*w+xx] = col
			}
		}
	}
	for i := range f.Y {
		v := int(f.Y[i]) + int(r.NormFloat64()*5)
		f.Y[i] = byte(clampInt(v, 0, 255))
	}
	return f
}

func cloneTestFrame(f *Frame) *Frame {
	return &Frame{
		W: f.W, H: f.H,
		Y: append([]byte(nil), f.Y...), R: append([]byte(nil), f.R...),
		G: append([]byte(nil), f.G...), B: append([]byte(nil), f.B...),
	}
}

func testTearFrame(base *Frame, split, dx int) *Frame {
	w, h := base.W, base.H
	out := &Frame{W: w, H: h, Y: make([]byte, w*h)}
	for y := 0; y < h; y++ {
		shift := 0
		if y >= split {
			shift = dx
		}
		for x := 0; x < w; x++ {
			sx := clampInt(x+shift, 0, w-1)
			out.Y[y*w+x] = base.Y[y*w+sx]
		}
	}
	return out
}

func testFieldFrame(base *Frame, dx int) *Frame {
	w, h := base.W, base.H
	out := &Frame{W: w, H: h, Y: make([]byte, w*h)}
	for y := 0; y < h; y++ {
		shift := 0
		if y%2 == 1 {
			shift = dx
		}
		for x := 0; x < w; x++ {
			sx := clampInt(x+shift, 0, w-1)
			out.Y[y*w+x] = base.Y[y*w+sx]
		}
	}
	return out
}

func testBlockFrame(base *Frame, bx, by, size int) *Frame {
	f := cloneTestFrame(base)
	r := rand.New(rand.NewSource(int64(bx*131 + by*17 + size)))
	for y := by; y < by+size && y < f.H; y++ {
		for x := bx; x < bx+size && x < f.W; x++ {
			i := y*f.W + x
			v := byte(r.Intn(256))
			f.Y[i], f.R[i], f.G[i], f.B[i] = v, v, v, v
		}
	}
	return f
}

func testFadeFrame(base *Frame, add int) *Frame {
	f := cloneTestFrame(base)
	for i := range f.Y {
		f.Y[i] = byte(clampInt(int(f.Y[i])+add, 0, 255))
	}
	return f
}

func testMovingFrame(base *Frame, x int) *Frame {
	f := cloneTestFrame(base)
	for y := 120; y < 250; y++ {
		for xx := x; xx < x+150 && xx < f.W; xx++ {
			if xx < 0 {
				continue
			}
			i := y*f.W + xx
			f.Y[i], f.R[i], f.G[i], f.B[i] = 225, 235, 90, 50
		}
	}
	return f
}

func analyzeFrames(frames []*Frame) Report {
	a := NewAnalyzer(DefaultOptions())
	for _, f := range frames {
		a.Process(f)
	}
	return a.Report("test")
}

func findTrack(rep Report, classification string) *Track {
	for i := range rep.Tracks {
		if rep.Tracks[i].Classification == classification {
			return &rep.Tracks[i]
		}
	}
	return nil
}

func TestStaticFrameNoFindings(t *testing.T) {
	base := testScene(640, 360, 1)
	frames := []*Frame{cloneTestFrame(base), cloneTestFrame(base), cloneTestFrame(base)}
	rep := analyzeFrames(frames)
	if rep.Summary.FramesWithFindings != 0 {
		t.Fatalf("static sequence produced findings: %+v", rep.Summary)
	}
}

func TestTearPositionAndContinuity(t *testing.T) {
	base := testScene(640, 360, 7)
	frames := make([]*Frame, 6)
	for i := range frames {
		frames[i] = testTearFrame(base, 170, 20)
	}
	rep := analyzeFrames(frames)
	if rep.Summary.TearFrames != 6 {
		t.Fatalf("tear frames = %d, want 6", rep.Summary.TearFrames)
	}
	tr := findTrack(rep, "tear")
	if tr == nil {
		t.Fatalf("missing tear track: %+v", rep.Tracks)
	}
	if tr.Continuity < 0.95 || tr.FrameCount != 6 {
		t.Fatalf("bad tear continuity: %+v", tr)
	}
	p := tr.Positions[0]
	if absInt(p.Y-170) > 8 {
		t.Fatalf("tear y = %d, want near 170", p.Y)
	}
	if absFloat(p.ShiftPx-20) > 5 {
		t.Fatalf("tear shift = %.1f, want near 20", p.ShiftPx)
	}
	if tr.Confidence < 0.65 {
		t.Fatalf("tear confidence too low: %.2f", tr.Confidence)
	}
}
