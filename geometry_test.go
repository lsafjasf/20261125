package main

import "testing"

func TestKnownDisplayMatrixClockwiseTurns(t *testing.T) {
	// Verified with FFmpeg 7/ffprobe: this matrix is printed as rotation=-90
	// and autorotate produces a 90-degree clockwise visual turn.
	raw := [9]int64{0, 65536, 0, -65536, 0, 0, 0, 0, 1073741824}
	m := matrixFromRaw(raw, "test")

	if m.TurnsClockwise != 1 {
		t.Fatalf("turns=%d, want 1; matrix=%+v", m.TurnsClockwise, m)
	}
	if m.DegreesClockwise != 90 || !m.SwapsAxes || !m.QuarterTurn {
		t.Fatalf("unexpected matrix interpretation: %+v", m)
	}
}

func TestFFProbeRotationSignConvention(t *testing.T) {
	cases := map[float64]int{
		-90:  1,
		-180: 2,
		180:  2,
		90:   3,
		0:    0,
	}
	for degrees, want := range cases {
		if got := turnsFromFFprobeRotation(degrees); got != want {
			t.Errorf("rotation %.0f => %d CW turns, want %d", degrees, got, want)
		}
	}
}

func TestRotateDims(t *testing.T) {
	cases := []struct {
		w, h, turns int
		wantW       int
		wantH       int
	}{
		{320, 180, 0, 320, 180},
		{320, 180, 1, 180, 320},
		{180, 320, 1, 320, 180},
		{320, 180, 2, 320, 180},
		{320, 180, 3, 180, 320},
	}
	for _, tc := range cases {
		gotW, gotH := rotateDims(tc.w, tc.h, tc.turns)
		if gotW != tc.wantW || gotH != tc.wantH {
			t.Errorf("rotateDims(%d,%d,%d)=%d,%d want %d,%d",
				tc.w, tc.h, tc.turns, gotW, gotH, tc.wantW, tc.wantH)
		}
	}
}

func TestParseFFProbeMatrixText(t *testing.T) {
	text := `"\n00000000:            0       65536           0\n00000001:       -65536           0           0\n00000002:            0           0  1073741824\n"`
	rawMessage := []byte(text)
	matrix, ok := parseFFProbeMatrix(rawMessage)
	if !ok {
		t.Fatal("parseFFProbeMatrix returned ok=false")
	}
	want := [9]int64{0, 65536, 0, -65536, 0, 0, 0, 0, 1073741824}
	if matrix != want {
		t.Fatalf("matrix=%v want %v", matrix, want)
	}
}

func TestParseRatio(t *testing.T) {
	n, d, ok := parseRatio("16:9")
	if !ok || n != 16 || d != 9 {
		t.Fatalf("16:9 => %d/%d/%v", n, d, ok)
	}
	if _, _, ok := parseRatio("0:1"); ok {
		t.Fatal("0:1 should be treated as unavailable")
	}
}
